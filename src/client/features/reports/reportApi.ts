import { apiRequest } from "@/client/lib/seomarineApi";
import { REPORT_APP_LIST_LIMIT } from "@/types/schemas/reports";
import { z } from "zod";

type ReportInput<T extends object = object> = {
  data: T & { projectId: string };
};

const metadataSchema = z.object({
  id: z.string(),
  projectId: z.string(),
  title: z.string(),
  summary: z.string(),
  skill: z.string().nullable(),
  templateId: z.string().nullable(),
  createdBy: z.string(),
  createdByUserId: z.string(),
  sizeBytes: z.number(),
  shareToken: z.string().nullable(),
  sharedAt: z.string().nullable(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
const templateSchema = z.object({
  id: z.string(),
  projectId: z.string(),
  name: z.string(),
  description: z.string(),
  instructions: z.string(),
  createdBy: z.string(),
  createdByUserId: z.string(),
  createdAt: z.string(),
  updatedAt: z.string(),
});

export type ReportListItem = z.infer<typeof metadataSchema> & {
  createdByName: string | null;
  templateName: string | null;
};
export type ReportTemplate = z.infer<typeof templateSchema>;

const reportPath = (projectId: string, suffix: string) =>
  `/api/v1/projects/${encodeURIComponent(projectId)}/reports/${suffix}`;

export async function listReports({
  data,
}: ReportInput<{ limit?: number; offset?: number }>) {
  const out = await apiRequest(
    reportPath(data.projectId, "list"),
    z.object({ reports: z.array(metadataSchema) }),
    "POST",
    {
      limit: Math.min(data.limit ?? REPORT_APP_LIST_LIMIT, 50),
      offset: data.offset ?? 0,
    },
  );
  const { templates } = await listReportTemplates({
    data: { projectId: data.projectId },
  });
  const templateNames = new Map(
    templates.map((template) => [template.id, template.name]),
  );
  return {
    reports: out.reports.map((report) => ({
      ...report,
      createdByName: null,
      templateName: report.templateId
        ? (templateNames.get(report.templateId) ?? null)
        : null,
    })),
  };
}

export async function getReport({ data }: ReportInput<{ reportId: string }>) {
  const out = await apiRequest(
    reportPath(data.projectId, "get"),
    z.object({ report: metadataSchema }),
    "POST",
    { reportId: data.reportId },
  );
  const { templates } = await listReportTemplates({
    data: { projectId: data.projectId },
  });
  const template = templates.find((item) => item.id === out.report.templateId);
  return {
    ...out.report,
    createdByName: null,
    templateName: template?.name ?? null,
  };
}

export async function shareReport({ data }: ReportInput<{ reportId: string }>) {
  return apiRequest(
    reportPath(data.projectId, "sharing"),
    z.object({
      reportId: z.string(),
      public: z.boolean(),
      shareToken: z.string().nullable(),
      sharedAt: z.string().nullable(),
    }),
    "POST",
    { reportId: data.reportId, public: true },
  );
}

export async function unshareReport({
  data,
}: ReportInput<{ reportId: string }>) {
  return apiRequest(
    reportPath(data.projectId, "sharing"),
    z.object({
      reportId: z.string(),
      public: z.boolean(),
      shareToken: z.string().nullable(),
      sharedAt: z.string().nullable(),
    }),
    "POST",
    { reportId: data.reportId, public: false },
  );
}

export async function deleteReport({
  data,
}: ReportInput<{ reportId: string }>) {
  return apiRequest(
    reportPath(data.projectId, "delete"),
    z.object({ reportId: z.string() }),
    "POST",
    { reportId: data.reportId },
  );
}

export async function listReportTemplates({ data }: ReportInput) {
  return apiRequest(
    reportPath(data.projectId, "templates/list"),
    z.object({ templates: z.array(templateSchema), remaining: z.number() }),
    "POST",
    {},
  );
}

export async function saveReportTemplate({
  data,
}: ReportInput<{
  templateId?: string;
  name: string;
  description: string;
  instructions: string;
}>) {
  try {
    const out = await apiRequest(
      reportPath(data.projectId, "templates/save"),
      z.object({
        templateId: z.string(),
        name: z.string(),
        created: z.boolean(),
      }),
      "POST",
      data,
    );
    return { ok: true as const, ...out };
  } catch (error) {
    if (
      error instanceof Error &&
      "code" in error &&
      error.code === "VALIDATION_ERROR"
    ) {
      return { ok: false as const, message: error.message };
    }
    throw error;
  }
}

export async function deleteReportTemplate({
  data,
}: ReportInput<{ templateId: string }>) {
  return apiRequest(
    reportPath(data.projectId, "templates/delete"),
    z.object({ templateId: z.string() }),
    "POST",
    { templateId: data.templateId },
  );
}
