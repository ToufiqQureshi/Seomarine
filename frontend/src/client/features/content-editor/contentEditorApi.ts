import { ApiError, apiRequest } from "@/client/lib/uplumoApi";
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
  authorId: z.string().optional(),
  // The review date arrives as an RFC 3339 midnight timestamp; the editor
  // only ever shows and sends its YYYY-MM-DD part.
  lastReviewedAt: z.iso.datetime({ offset: true }).optional(),
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
const uploadedImageSchema = z.object({ url: z.string().url() });

export type ContentDraft = z.infer<typeof draftSchema>;

export const contentEditorQueryKeys = {
  drafts: (projectId: string) => ["contentDrafts", projectId] as const,
  draft: (projectId: string, draftId: string) =>
    ["contentDraft", projectId, draftId] as const,
};

export function getContentWriterStatus() {
  return apiRequest(
    "/api/v1/config/content-writer-status",
    z.object({
      configured: z.boolean(),
      message: z.string(),
      maxCreditsPerAction: z.number().int().positive(),
    }),
    "GET",
  );
}
export type ContentDraftSummary = z.infer<typeof draftSummarySchema>;

type WriterAction =
  | "outline"
  | "draft"
  | "improve"
  | "rewrite"
  | "faq"
  | "write_section"
  | "fill_gap"
  | "answer_first"
  | "fix_top_issue"
  | "title_meta"
  | "destuff"
  | "fact_check"
  | "tone_language";
type WriterRequest = Pick<
  ContentDraftInput,
  "title" | "targetKeyword" | "content"
> & {
  projectId: string;
  action: WriterAction;
  mode: "seo" | "aeo";
  language: string;
  instructions: string;
  /** Terms, questions and competitor headings from the search brief. */
  brief: string;
  idempotencyKey: string;
};

const searchBriefSchema = z.object({
  keyword: z.string(),
  locationCode: z.number().int(),
  languageCode: z.string(),
  fetchedAt: z.string(),
  questions: z
    .array(z.string())
    .nullable()
    .transform((v) => v ?? []),
  competitors: z
    .array(
      z.object({
        url: z.string(),
        title: z.string(),
        headings: z
          .array(z.string())
          .nullable()
          .transform((v) => v ?? []),
        wordCount: z.number().int().nonnegative(),
        text: z.string(),
      }),
    )
    .nullable()
    .transform((v) => v ?? []),
  terms: z
    .array(
      z.object({
        term: z.string(),
        min: z.number().int().nonnegative(),
        max: z.number().int().nonnegative(),
        domains: z.number().int().nonnegative(),
      }),
    )
    .nullable()
    .transform((v) => v ?? []),
  subtopics: z
    .array(z.string())
    .nullable()
    .transform((v) => v ?? []),
});

export type SearchBrief = z.infer<typeof searchBriefSchema>;

/** Builds or reads the shared search brief. A cache miss or refresh costs credits. */
export function fetchSearchBrief({
  data,
}: {
  data: { projectId: string; keyword: string; refresh: boolean };
}) {
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/content/brief`,
    searchBriefSchema,
    "POST",
    { keyword: data.keyword, refresh: data.refresh },
  );
}

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

export async function uploadContentImage({
  data,
}: {
  data: { projectId: string; file: File };
}) {
  const form = new FormData();
  form.append("image", data.file, data.file.name);
  const response = await fetch(
    `/api/v1/projects/${encodeURIComponent(data.projectId)}/content/images/upload`,
    { method: "POST", credentials: "same-origin", body: form },
  );
  const body: unknown = await response.json().catch(() => undefined);
  if (!response.ok) {
    const parsedError = z
      .object({ error: z.object({ code: z.string(), message: z.string() }) })
      .safeParse(body);
    throw new ApiError(
      response.status,
      parsedError.success ? parsedError.data.error.code : "unknown",
      parsedError.success
        ? parsedError.data.error.message
        : "Could not upload this image. Try again.",
    );
  }
  const parsed = uploadedImageSchema.safeParse(body);
  if (!parsed.success) {
    throw new ApiError(
      0,
      "invalid_response",
      "The server returned an unexpected image URL. Try again.",
    );
  }
  return parsed.data;
}
