import { apiRequest } from "@/client/lib/seomarineApi";
import { z } from "zod";

const usageEventSchema = z.object({
  timestamp: z.number(),
  value: z.number(),
  properties: z.record(z.string(), z.json()),
});

export type BillingUsageEvent = z.infer<typeof usageEventSchema>;

export async function getBillingUsageEvents({
  data,
}: {
  data: { start: number; end: number };
}) {
  return apiRequest(
    "/api/v1/billing/usage-events",
    z.array(usageEventSchema),
    "POST",
    data,
  );
}
