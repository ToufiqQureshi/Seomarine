import { apiRequest } from "@/client/lib/uplumoApi";
import { z } from "zod";
import type { ContentDraftInput } from "./scoring";

const draftSchema = z.object({
  id: z.string(),
  projectId: z.string(),
  title: z.string(),
  targetKeyword: z.string(),
  slug: z.string(),
  metaDescription: z.string(),
  content: z.string(),
  createdAt: z.string(),
  updatedAt: z.string(),
});
const draftSummarySchema = draftSchema.pick({
  id: true,
  projectId: true,
  title: true,
  targetKeyword: true,
  updatedAt: true,
});

export type ContentDraft = z.infer<typeof draftSchema>;
export type ContentDraftSummary = z.infer<typeof draftSummarySchema>;

type WriterAction = "outline" | "draft" | "improve" | "rewrite" | "faq";
type WriterRequest = Pick<
  ContentDraftInput,
  "title" | "targetKeyword" | "content"
> & {
  projectId: string;
  action: WriterAction;
  mode: "seo" | "aeo";
  language: string;
  instructions: string;
};

export function listContentDrafts({ data }: { data: { projectId: string } }) {
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/content/drafts/list`,
    z.array(draftSummarySchema),
    "POST",
    {},
  );
}

export function getContentDraft({
  data,
}: {
  data: { projectId: string; draftId: string };
}) {
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/content/drafts/${encodeURIComponent(data.draftId)}/get`,
    draftSchema,
    "POST",
    {},
  );
}

export function saveContentDraft({
  data,
}: {
  data: ContentDraftInput & { id?: string; projectId: string };
}) {
  const { projectId, ...input } = data;
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(projectId)}/content/drafts/save`,
    draftSchema,
    "POST",
    input,
  );
}

export function deleteContentDraft({
  data,
}: {
  data: { projectId: string; draftId: string };
}) {
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/content/drafts/${encodeURIComponent(data.draftId)}/delete`,
    z.object({ success: z.boolean() }),
    "POST",
    {},
  );
}

export function generateWritingSuggestion({ data }: { data: WriterRequest }) {
  const { projectId, ...input } = data;
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(projectId)}/content/assistant/generate`,
    z.object({ content: z.string().min(1) }),
    "POST",
    input,
  );
}
