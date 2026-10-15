import { beforeEach, describe, expect, it, vi } from "vitest";
import type { MockInstance } from "vitest";
import {
  deleteContentDraft,
  getContentDraft,
  generateWritingSuggestion,
  listContentDrafts,
  saveContentDraft,
  uploadContentImage,
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
          brief: "Terms: channel manager",
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
          brief: "Terms: channel manager",
        }),
      },
    ]);
  });

  it("uploads one image as multipart form data and validates the public URL", async () => {
    fetchMock.mockResolvedValue(
      Response.json(
        { url: "https://media.uplumo.com/content-editor/a.png" },
        { status: 201 },
      ),
    );
    const file = new File(["image bytes"], "cover.webp", {
      type: "image/webp",
    });
    await expect(
      uploadContentImage({ data: { projectId: "p/1", file } }),
    ).resolves.toEqual({
      url: "https://media.uplumo.com/content-editor/a.png",
    });
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/projects/p%2F1/content/images/upload");
    expect(init).toMatchObject({ method: "POST", credentials: "same-origin" });
    expect(init?.headers).toBeUndefined();
    const body = init?.body;
    if (!(body instanceof FormData)) {
      throw new Error("Expected the upload request to contain FormData.");
    }
    expect(body.get("image")).toBeInstanceOf(File);
  });

  it("shows safe upload errors returned by the server", async () => {
    fetchMock.mockResolvedValue(
      Response.json(
        {
          error: {
            code: "invalid_image",
            message: "Choose a PNG or JPEG image.",
          },
        },
        { status: 400 },
      ),
    );
    const file = new File(["not an image"], "cover.webp", {
      type: "image/webp",
    });
    await expect(
      uploadContentImage({ data: { projectId: "p1", file } }),
    ).rejects.toMatchObject({ status: 400, code: "invalid_image" });
  });
});
