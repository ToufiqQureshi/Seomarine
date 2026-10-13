import { apiRequest } from "@/client/lib/uplumoApi";
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

const creditsSchema = z.object({
  plan: z.string(),
  status: z.string(),
  currentPeriodEnd: z.string().nullable(),
  balanceCredits: z.number(),
  balanceUsd: z.number(),
  monthlyGrantCredits: z.number(),
  paymentsConfigured: z.boolean(),
});

export type BillingCredits = z.infer<typeof creditsSchema>;

export function getBillingCredits() {
  return apiRequest("/api/v1/billing/credits", creditsSchema);
}
