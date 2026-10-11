import { z } from "zod";

const dashboardProjectInputSchema = z.object({
  projectId: z.string().min(1),
});

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
const dashboardStepClickSchema = dashboardProjectInputSchema.extend({
  step: dashboardClickStepSchema,
});
const dashboardStepDismissalSchema = dashboardProjectInputSchema.extend({
  step: dashboardSetupStepSchema,
  dismissed: z.boolean(),
});
