import { beforeEach, describe, expect, it, vi } from "vitest";
import type { MockInstance } from "vitest";
import {
  deleteContentDraft,
  getContentDraft,
  generateWritingSuggestion,
  listContentDrafts,
  saveContentDraft,
} from "./contentEditorApi";

const draft = {
  id: "draft-1",
  projectId: "project-1",
  title: "SEO guide",
  targetKeyword: "technical SEO",
  slug: "technical-seo",
  metaDescription: "A useful SEO guide",
  content: "Draft content",
  createdAt: "2026-10-12T12:00:00Z",
  updatedAt: "2026-10-12T12:00:00Z",
};

let fetchMock: MockInstance<typeof fetch>;

beforeEach(() => {
  fetchMock = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(Response.json(draft));
});

describe("content editor API contract", () => {
  it("lists project-scoped drafts and rejects malformed responses", async () => {
    fetchMock.mockResolvedValue(Response.json([{ ...draft, id: 42 }]));

    await expect(
      listContentDrafts({ data: { projectId: "p/1" } }),
    ).rejects.toMatchObject({
      status: 0,
      code: "invalid_response",
    });
    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/projects/p%2F1/content/drafts/list",
      { method: "POST", body: "{}" },
    ]);
  });

  it("saves a draft without leaking the project path into JSON", async () => {
    await expect(
      saveContentDraft({
        data: {
          projectId: "p/1",
          id: "draft-1",
          title: draft.title,
          targetKeyword: draft.targetKeyword,
          slug: draft.slug,
          metaDescription: draft.metaDescription,
          content: draft.content,
        },
      }),
    ).resolves.toEqual(draft);
    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/projects/p%2F1/content/drafts/save",
      {
        method: "POST",
        body: JSON.stringify({
          id: draft.id,
          title: draft.title,
          targetKeyword: draft.targetKeyword,
          slug: draft.slug,
          metaDescription: draft.metaDescription,
          content: draft.content,
        }),
      },
    ]);
  });

  it("loads one project-scoped draft, including its full article body", async () => {
    await expect(
      getContentDraft({ data: { projectId: "p1", draftId: "draft-1" } }),
    ).resolves.toEqual(draft);
    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/projects/p1/content/drafts/draft-1/get",
      { method: "POST", body: "{}" },
    ]);
  });

  it("deletes an encoded draft ID and validates the response", async () => {
    fetchMock.mockResolvedValue(Response.json({ success: true }));
    await expect(
      deleteContentDraft({ data: { projectId: "p1", draftId: "draft/1" } }),
    ).resolves.toEqual({ success: true });
    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/projects/p1/content/drafts/draft%2F1/delete",
      { method: "POST", body: "{}" },
    ]);
  });

  it("sends a scoped writing action and validates the generated suggestion", async () => {
    fetchMock.mockResolvedValue(Response.json({ content: "## Draft outline" }));
    await expect(
      generateWritingSuggestion({
        data: {
          projectId: "p/1",
          action: "outline",
          mode: "aeo",
          title: "SEO guide",
          targetKeyword: "technical SEO",
          content: "Existing facts",
          language: "English",
          instructions: "Be concise",
        },
      }),
    ).resolves.toEqual({ content: "## Draft outline" });
    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/projects/p%2F1/content/assistant/generate",
      {
        method: "POST",
        body: JSON.stringify({
          action: "outline",
          mode: "aeo",
          title: "SEO guide",
          targetKeyword: "technical SEO",
          content: "Existing facts",
          language: "English",
          instructions: "Be concise",
        }),
      },
    ]);
  });
});
