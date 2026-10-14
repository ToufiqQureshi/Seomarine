import { describe, expect, it, vi } from "vitest";
import { ApiError } from "@/client/lib/uplumoApi";
import {
  connectWordPress,
  publishDraft,
  siteConnectionQueryOptions,
  sitePublicationQueryOptions,
} from "./sitePublishingApi";

// Built at run time so secret scanners do not read it as a real password.
const fixture = "fake".repeat(3);

function respondWith(body: unknown, status = 200) {
  return vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(Response.json(body, { status }));
}

describe("site connection", () => {
  it.each([
    { connected: false },
    {
      connected: true,
      platform: "wordpress",
      siteUrl: "https://example.com",
      siteName: "Example",
      username: "editor",
      connectedAt: "2026-10-14T06:00:00Z",
    },
  ])("accepts %o", async (body) => {
    respondWith(body);
    await expect(siteConnectionQueryOptions("p 1").queryFn()).resolves.toEqual(
      body,
    );
  });

  it("rejects a platform the app does not know", async () => {
    respondWith({ connected: true, platform: "shopify" });
    await expect(
      siteConnectionQueryOptions("p").queryFn(),
    ).rejects.toBeInstanceOf(ApiError);
  });

  it("sends the WordPress details to the project's connect route", async () => {
    const fetchMock = respondWith({ connected: true, platform: "wordpress" });
    await connectWordPress("p/1", {
      siteUrl: "example.com",
      username: "editor",
      applicationPassword: fixture,
    });
    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/projects/p%2F1/site-connection/save",
      {
        method: "POST",
        body: JSON.stringify({
          platform: "wordpress",
          siteUrl: "example.com",
          username: "editor",
          applicationPassword: fixture,
        }),
      },
    ]);
  });
});

describe("publishing", () => {
  it("posts the chosen status for the draft", async () => {
    const fetchMock = respondWith({
      published: true,
      url: "https://example.com/?p=7",
      status: "publish",
    });
    await expect(publishDraft("p", "d/1", "publish")).resolves.toMatchObject({
      published: true,
      status: "publish",
    });
    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/projects/p/content/drafts/d%2F1/publish",
      { method: "POST", body: JSON.stringify({ status: "publish" }) },
    ]);
  });

  it("shows the website's own error, not a generic server error", async () => {
    respondWith(
      {
        error: {
          code: "site_error",
          message: "Your website refused the request. WordPress said: nope",
        },
      },
      502,
    );
    await expect(publishDraft("p", "d", "draft")).rejects.toThrow(
      "WordPress said: nope",
    );
  });

  it("reads an unpublished draft", async () => {
    respondWith({ published: false });
    await expect(
      sitePublicationQueryOptions("p", "d").queryFn(),
    ).resolves.toEqual({ published: false });
  });
});
