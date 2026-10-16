import type { ContentDraftInput } from "./scoring";

export const titleForDraft = (draft: ContentDraftInput) =>
  draft.title.trim() ||
  draft.content.match(/^#\s+(.+)$/mu)?.[1]?.trim() ||
  "Untitled draft";

export const searchPreviewTitle = (draft: ContentDraftInput) =>
  draft.title.trim() || draft.content.match(/^#\s+(.+)$/mu)?.[1]?.trim() || "";

export const searchPreviewUrl = (domain: string, slug: string) =>
  domain
    ? `https://${domain}/${slug || "your-page"}`
    : "Connect a website to preview its URL";
