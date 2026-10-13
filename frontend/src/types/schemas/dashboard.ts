import { z } from "zod";

export const dashboardSetupStepSchema = z.enum([
  "competitor",
  "keywords",
  "audit",
  "mcp",
  "team",
  "project",
]);
export type DashboardSetupStep = z.infer<typeof dashboardSetupStepSchema>;
// Steps that complete on click-through rather than from product state.
export const dashboardClickStepSchema = z.enum(["competitor", "keywords"]);
export type DashboardClickStep = z.infer<typeof dashboardClickStepSchema>;
