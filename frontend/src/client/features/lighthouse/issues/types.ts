import type { z } from "zod";
import type { LighthouseIssuesData } from "@/client/features/audit/auditApi";
import {
  LIGHTHOUSE_CATEGORY_TABS,
  type LighthouseCategoryTab,
} from "@/shared/lighthouse";
import type { lighthouseAuditExportSchema } from "@/types/schemas/lighthouse";

export const categoryTabs = LIGHTHOUSE_CATEGORY_TABS;

export type CategoryTab = LighthouseCategoryTab;

export type ExportPayload = Omit<
  z.infer<typeof lighthouseAuditExportSchema>,
  "projectId" | "resultId"
>;

export type LighthouseIssue = LighthouseIssuesData["issues"][number];
export type LighthouseScores = NonNullable<LighthouseIssuesData["scores"]>;
export type LighthouseMetrics = NonNullable<LighthouseIssuesData["metrics"]>;
