import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/client/lib/seomarineApi";
import { listSamMessages, rewindSamSession, samChatUrl } from "./samApi";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("listSamMessages", () => {
  it("posts the session id to the project's endpoint and returns the stored messages", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json([
        { id: "m1", role: "user", parts: [{ type: "text", text: "hi" }] },
        {
          id: "m2",
          role: "assistant",
          parts: [
            { type: "step-start" },
            {
              type: "tool-get_serp_results",
              toolCallId: "c1",
              state: "output-available",
              input: { keyword: "anvils" },
              output: { summary: "ok" },
            },
          ],
        },
      ]),
    );

    const messages = await listSamMessages({
      data: { projectId: "p/1", sessionId: "s1" },
    });

    expect(messages.map((m) => m.id)).toEqual(["m1", "m2"]);
    expect(messages[1].parts[1]).toMatchObject({ toolCallId: "c1" });
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/projects/p%2F1/sam/sessions/messages");
    expect(JSON.parse(String(init?.body))).toEqual({ sessionId: "s1" });
  });

  it("rejects a response that is not a message list", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json([{ id: "m1", role: "system", parts: [] }]),
    );
    await expect(
      listSamMessages({ data: { projectId: "p1", sessionId: "s1" } }),
    ).rejects.toBeInstanceOf(ApiError);
  });
});

describe("rewindSamSession", () => {
  it("sends the session and the message to rewind to", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json({ ok: true }));
    await rewindSamSession({
      data: { projectId: "p1", sessionId: "s1", messageId: "m9" },
    });
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/projects/p1/sam/sessions/rewind");
    expect(JSON.parse(String(init?.body))).toEqual({
      sessionId: "s1",
      messageId: "m9",
    });
  });

  it("surfaces a server refusal as an ApiError", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json(
        {
          error: {
            code: "sam_turn_in_progress",
            message: "SAM is still stopping.",
          },
        },
        { status: 409 },
      ),
    );
    await expect(
      rewindSamSession({
        data: { projectId: "p1", sessionId: "s1", messageId: "m1" },
      }),
    ).rejects.toMatchObject({ status: 409, code: "sam_turn_in_progress" });
  });
});

describe("samChatUrl", () => {
  it("encodes the project id", () => {
    expect(samChatUrl("a/b")).toBe("/api/v1/projects/a%2Fb/sam/sessions/chat");
  });
});
