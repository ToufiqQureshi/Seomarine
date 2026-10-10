import { apiRequest } from "@/client/lib/seomarineApi";
import { z } from "zod";

export async function getSeoApiKeyStatus() {
  return apiRequest(
    "/api/v1/config/seo-api-key-status",
    z.object({ configured: z.boolean() }),
    "GET",
  );
}
