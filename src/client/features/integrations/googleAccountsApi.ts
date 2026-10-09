import { z } from "zod";
import { apiRequest } from "@/client/lib/seomarineApi";

const impactSchema = z.object({ projectCount: z.number().int().nonnegative() });
const removalSchema = z.object({ removed: z.literal(true) });

type Account = { provider: "gsc" | "ga4"; accountId: string };

export function getGoogleAccountRemovalImpact(account: Account) {
  return apiRequest(
    "/api/v1/google/accounts/impact",
    impactSchema,
    "POST",
    account,
  );
}

export function removeGoogleAccount(account: Account) {
  return apiRequest("/api/v1/google/accounts/remove", removalSchema, "POST", {
    ...account,
    confirmed: true,
  });
}
