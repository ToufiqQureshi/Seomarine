import { apiRequest } from "@/client/lib/seomarineApi";
import { z } from "zod";

const credentialSchema = z.object({
  id: z.string(),
  projectId: z.string(),
  host: z.string(),
  provider: z.literal("shopify"),
  createdAt: z.string(),
  expiresAt: z.string().nullable(),
});

const signatureProblemSchema = z.discriminatedUnion("reason", [
  z.object({
    reason: z.literal("wrong_domain"),
    host: z.string(),
    signedHost: z.string(),
  }),
  z.object({ reason: z.literal("invalid"), host: z.string() }),
]);

const saveResultSchema = z.union([
  z.object({ credential: credentialSchema }),
  z.object({ problem: signatureProblemSchema }),
]);

export function listCrawlerCredentials() {
  return apiRequest(
    "/api/v1/crawler-access/credentials",
    z.array(credentialSchema),
    "GET",
  );
}

export function saveCrawlerCredential({
  data,
}: {
  data: {
    projectId: string;
    host: string;
    signatureInput: string;
    signature: string;
  };
}) {
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/crawler-access/save`,
    saveResultSchema,
    "POST",
    {
      host: data.host,
      signatureInput: data.signatureInput,
      signature: data.signature,
    },
  );
}

export function deleteCrawlerCredential({ data }: { data: { id: string } }) {
  return apiRequest(
    "/api/v1/crawler-access/delete",
    z.object({ success: z.literal(true) }),
    "POST",
    data,
  );
}
