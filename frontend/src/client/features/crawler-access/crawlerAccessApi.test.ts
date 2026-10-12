import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/client/lib/uplumoApi";
import {
  deleteCrawlerCredential,
  listCrawlerCredentials,
  saveCrawlerCredential,
} from "./crawlerAccessApi";

afterEach(() => vi.restoreAllMocks());

describe("crawler access API", () => {
  it("lists credentials and validates returned provider data", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json([
        {
          id: "credential-1",
          projectId: "p1",
          host: "shop.example",
          provider: "shopify",
          createdAt: "now",
          expiresAt: null,
        },
      ]),
    );
    await expect(listCrawlerCredentials()).resolves.toMatchObject([
      { id: "credential-1", host: "shop.example" },
    ]);
    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/crawler-access/credentials",
      { method: "GET" },
    ]);
  });

  it("encodes project IDs and only sends signed credential fields", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({
        problem: {
          reason: "wrong_domain",
          host: "shop.example",
          signedHost: "other.example",
        },
      }),
    );
    await expect(
      saveCrawlerCredential({
        data: {
          projectId: "p/1",
          host: "shop.example",
          signatureInput: "signed-data",
          signature: "sig",
        },
      }),
    ).resolves.toMatchObject({ problem: { reason: "wrong_domain" } });
    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/projects/p%2F1/crawler-access/save",
      {
        method: "POST",
        body: JSON.stringify({
          host: "shop.example",
          signatureInput: "signed-data",
          signature: "sig",
        }),
      },
    ]);
  });

  it("rejects malformed provider responses and confirms deletion", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(Response.json({ credential: { id: 4 } }));
    await expect(
      saveCrawlerCredential({
        data: {
          projectId: "p1",
          host: "shop.example",
          signatureInput: "data",
          signature: "sig",
        },
      }),
    ).rejects.toBeInstanceOf(ApiError);
    fetchMock.mockResolvedValueOnce(Response.json({ success: true }));
    await expect(
      deleteCrawlerCredential({ data: { id: "credential-1" } }),
    ).resolves.toEqual({ success: true });
    expect(fetchMock.mock.calls[1]).toMatchObject([
      "/api/v1/crawler-access/delete",
      { method: "POST", body: JSON.stringify({ id: "credential-1" }) },
    ]);
  });
});
