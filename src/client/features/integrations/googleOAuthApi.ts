import { z } from "zod";
import { apiRequest } from "@/client/lib/seomarineApi";
import type { GoogleLinkProvider } from "@/shared/google-link";

const authorizationSchema = z.object({ url: z.url() });

export function startGoogleOAuth(
  provider: GoogleLinkProvider,
  callbackURL: string,
) {
  return apiRequest(
    `/api/v1/google/oauth/${provider}/start`,
    authorizationSchema,
    "POST",
    { callbackURL },
  );
}
