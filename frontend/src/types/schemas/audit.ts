import { z } from "zod";

// The audit server functions moved to the Go API
// (`src/client/features/audit/auditApi.ts`), so their input schemas are gone.
// This file now only holds the URL search-params schema for the audit route,
// which the React router still reads.

const auditTabs = ["issues", "pages", "performance"] as const;

export const auditSearchSchema = z.object({
  auditId: z.string().optional().catch(undefined),
  // Pre-fills the launch form (the dashboard's "Audit your site" step).
  url: z.string().optional().catch(undefined),
  tab: z.enum(auditTabs).catch("issues").default("issues"),
});
