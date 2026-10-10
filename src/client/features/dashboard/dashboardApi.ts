import { apiRequest } from "@/client/lib/seomarineApi";
import type {
  DashboardClickStep,
  DashboardSetupStep,
} from "@/types/schemas/dashboard";
import { z } from "zod";

const projectInput = { projectId: z.string().min(1) };
type ProjectInput = { data: { projectId: string } };

const dashboardActivationSchema = z.object({
  domain: z.string().nullable(),
  ga4: z.object({
    connected: z.boolean(),
    propertyDisplayName: z.string().nullable(),
    cardDismissedAt: z.string().nullable(),
  }),
  gsc: z.object({ connected: z.boolean(), siteUrl: z.string().nullable() }),
  mcp: z.object({
    authorizedAt: z.string().nullable(),
    firstToolCallAt: z.string().nullable(),
    cardDismissedAt: z.string().nullable(),
  }),
  competitorClickedAt: z.string().nullable(),
  keywordsClickedAt: z.string().nullable(),
  hasAudit: z.boolean(),
  hasMultipleProjects: z.boolean(),
  hasTeammate: z.boolean(),
  dismissedSteps: z.array(z.string()),
});

const dashboardAuditSchema = z.object({
  status: z.enum(["running", "completed", "failed"]),
  pagesCrawled: z.number(),
  startedAt: z.string(),
  topIssues: z.array(
    z.object({
      issueType: z.string(),
      severity: z.enum(["critical", "warning", "info"]),
      count: z.number(),
    }),
  ),
  totalIssueTypes: z.number(),
});

const dashboardBacklinksSchema = z.object({
  domain: z.string(),
  rank: z.number().nullable(),
  backlinks: z.number().nullable(),
  referringDomains: z.number().nullable(),
  newBacklinks: z.number().nullable(),
  lostBacklinks: z.number().nullable(),
  newReferringDomains: z.number().nullable(),
  lostReferringDomains: z.number().nullable(),
  capturedAt: z.string(),
  stale: z.boolean(),
});

export type DashboardActivation = z.infer<typeof dashboardActivationSchema>;
export type DashboardOverview = {
  audit: z.infer<typeof dashboardAuditSchema> | null;
  backlinks: z.infer<typeof dashboardBacklinksSchema> | null;
};

export async function getDashboardActivation({ data }: ProjectInput) {
  projectInput.projectId.parse(data.projectId);
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/activation/get`,
    dashboardActivationSchema,
    "POST",
    {},
  );
}

export async function getDashboardOverview({ data }: ProjectInput) {
  projectInput.projectId.parse(data.projectId);
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/dashboard/overview`,
    z.object({
      audit: dashboardAuditSchema.nullable(),
      backlinks: dashboardBacklinksSchema.nullable(),
    }),
    "POST",
    {},
  );
}

export async function markDashboardStepClicked({
  data,
}: {
  data: { projectId: string; step: DashboardClickStep };
}) {
  projectInput.projectId.parse(data.projectId);
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/activation/click`,
    z.object({ ok: z.literal(true) }),
    "POST",
    { step: data.step },
  );
}

export async function dismissDashboardGa4Card({ data }: ProjectInput) {
  projectInput.projectId.parse(data.projectId);
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/activation/ga4-dismiss`,
    z.object({ ok: z.literal(true) }),
    "POST",
    {},
  );
}

export async function setDashboardStepDismissed({
  data,
}: {
  data: { projectId: string; step: DashboardSetupStep; dismissed: boolean };
}) {
  projectInput.projectId.parse(data.projectId);
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/activation/dismiss`,
    z.object({ ok: z.literal(true) }),
    "POST",
    { step: data.step, dismissed: data.dismissed },
  );
}
