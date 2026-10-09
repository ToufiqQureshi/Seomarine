import { symmetricDecrypt, symmetricEncrypt } from "better-auth/crypto";
import { and, eq } from "drizzle-orm";
import { z } from "zod";
import { db } from "@/db";
import { account } from "@/db/schema";
import { getAuth } from "@/lib/auth";
import { AppError } from "@/server/lib/errors";
import { getGoogleOAuthClientConfig } from "./oauth-config";

/**
 * Google data grants (Search Console, Analytics) are linked here rather than
 * through Better Auth's generic-oauth plugin. Better Auth treats every linked
 * provider account as a login identity and refuses to link one Google account
 * to a second user, which blocked agencies and multi-member orgs from each
 * connecting the same client Google account. Grants live in the Better Auth
 * `account` table, encrypted with its secret, but are keyed per user so the
 * same Google identity may back several users.
 *
 * The consent redirect and callback now live in Go
 * (`backend/internal/google`): this module only refreshes and reads the
 * stored grants until the GSC/GA4 data APIs are ported.
 */

const GOOGLE_TOKEN_URL = "https://oauth2.googleapis.com/token";
// Refresh when the stored access token is within this many ms of expiry.
const ACCESS_TOKEN_SKEW_MS = 5_000;

const googleTokenResponseSchema = z.object({
  access_token: z.string().min(1),
  expires_in: z.number().optional(),
  refresh_token: z.string().optional(),
  scope: z.string().optional(),
  id_token: z.string().optional(),
});

/** Token encryption matches Better Auth's own `account` rows so grants
 *  written before this module (and google social-login rows) stay readable. */
async function tokenCrypto() {
  const ctx = await getAuth().$context;
  const enabled = Boolean(ctx.options.account?.encryptOAuthTokens);
  return {
    encrypt: (value: string) =>
      enabled
        ? symmetricEncrypt({ key: ctx.secretConfig, data: value })
        : value,
    decrypt: (value: string) =>
      enabled
        ? symmetricDecrypt({ key: ctx.secretConfig, data: value })
        : value,
  };
}

/** Google's token endpoint; null when it rejects the grant. */
async function fetchTokens(params: Record<string, string>) {
  const response = await fetch(GOOGLE_TOKEN_URL, {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: new URLSearchParams(params),
  });
  if (!response.ok) return null;
  return googleTokenResponseSchema.parse(await response.json());
}

/**
 * A currently valid access token for one of the user's Google grants,
 * refreshed against Google and re-persisted when it is about to expire.
 * `accountId` (the Google sub) picks between several grants for the same
 * provider; legacy connections without one fall back to the first grant.
 */
export async function getGoogleAccessToken(input: {
  userId: string;
  providerId: string;
  accountId?: string;
}): Promise<string> {
  const [grant] = await db
    .select({
      id: account.id,
      accessToken: account.accessToken,
      refreshToken: account.refreshToken,
      accessTokenExpiresAt: account.accessTokenExpiresAt,
    })
    .from(account)
    .where(
      and(
        eq(account.userId, input.userId),
        eq(account.providerId, input.providerId),
        input.accountId ? eq(account.accountId, input.accountId) : undefined,
      ),
    )
    .limit(1);
  if (!grant) throw new Error(`No ${input.providerId} grant for this user.`);
  const { encrypt, decrypt } = await tokenCrypto();
  const expiresAt = grant.accessTokenExpiresAt?.getTime();
  const fresh =
    expiresAt !== undefined && expiresAt - Date.now() > ACCESS_TOKEN_SKEW_MS;
  if (fresh && grant.accessToken) return decrypt(grant.accessToken);
  if (!grant.refreshToken) {
    // Google only issues a refresh token on the first consent, so a grant
    // without one cannot recover once its access token expires. Failing here
    // lets the GSC/GA4 clients ask for a reconnect instead of sending a dead
    // token to Google.
    throw new Error(
      `The ${input.providerId} grant has expired and cannot be refreshed.`,
    );
  }
  const config = await getGoogleOAuthClientConfig();
  if (!config) {
    throw new AppError(
      "AUTH_CONFIG_MISSING",
      "Google OAuth is not configured.",
    );
  }
  const tokens = await fetchTokens({
    client_id: config.clientId,
    client_secret: config.clientSecret,
    refresh_token: await decrypt(grant.refreshToken),
    grant_type: "refresh_token",
  });
  if (!tokens) {
    throw new Error(`Google refused to refresh the ${input.providerId} grant.`);
  }
  await db
    .update(account)
    .set({
      accessToken: await encrypt(tokens.access_token),
      accessTokenExpiresAt: new Date(
        Date.now() + (tokens.expires_in ?? 3600) * 1_000,
      ),
      // Google rarely rotates refresh tokens; keep a new one when it does.
      ...(tokens.refresh_token
        ? { refreshToken: await encrypt(tokens.refresh_token) }
        : {}),
      updatedAt: new Date(),
    })
    .where(eq(account.id, grant.id));
  return tokens.access_token;
}
