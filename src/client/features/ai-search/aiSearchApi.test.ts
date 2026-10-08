import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/client/lib/seomarineApi";
import { explorePrompt, lookupBrand } from "./aiSearchApi";

afterEach(() => {
  vi.restoreAllMocks();
});

const lookupResult = {
  query: "acme.com",
  detectedTargetType: "domain",
  resolvedTarget: "acme.com",
  scope: "subdomains",
  aggregatesAreDomainLevel: false,
  fetchedAt: "2026-10-08T09:00:00.000Z",
  hasData: true,
  totalMentions: 5,
  totalAiSearchVolume: 50,
  perPlatform: [
    { platform: "google", status: "success", mentions: 5, aiSearchVolume: 50 },
  ],
  shareOfVoice: null,
  topPages: [],
  topQueries: [],
  monthlyVolume: [],
};

describe("lookupBrand", () => {
  it("posts to the project's endpoint without repeating the project id in the body", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json(lookupResult));

    const result = await lookupBrand({
      projectId: "p/1",
      query: "acme.com",
      competitors: ["rival.com"],
      scope: "domain",
      locationCode: 2840,
      languageCode: "en",
    });

    expect(result.totalMentions).toBe(5);
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/projects/p%2F1/ai-search/brand-lookup");
    expect(init?.method).toBe("POST");
    expect(init?.body).toBe(
      JSON.stringify({
        query: "acme.com",
        competitors: ["rival.com"],
        scope: "domain",
        locationCode: 2840,
        languageCode: "en",
      }),
    );
  });

  it("rejects a response that breaks the contract", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({ ...lookupResult, hasData: "yes" }),
    );
    const err = await lookupBrand({
      projectId: "p1",
      query: "acme.com",
      competitors: [],
      locationCode: 2840,
      languageCode: "en",
    }).catch((e: unknown) => e);
    expect(err).toMatchObject({ status: 0, code: "invalid_response" });
  });

  it("tells a free-plan user to upgrade", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json(
        {
          error: {
            code: "payment_required",
            message: "Upgrade to the paid plan to use AI Visibility.",
          },
        },
        { status: 402 },
      ),
    );
    const err = await lookupBrand({
      projectId: "p1",
      query: "acme.com",
      competitors: [],
      locationCode: 2840,
      languageCode: "en",
    }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({
      status: 402,
      message: "Upgrade to the paid plan to use AI Visibility.",
    });
  });
});

describe("explorePrompt", () => {
  it("reads a mix of answers and per-model errors", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({
        prompt: "hi",
        highlightBrand: null,
        fetchedAt: "2026-10-08T09:00:00.000Z",
        results: [
          {
            status: "success",
            model: "chat_gpt",
            modelName: "gpt-5",
            text: "answer",
            citations: [
              {
                url: "https://example.com/a",
                domain: "example.com",
                title: null,
                matchedBrand: false,
              },
            ],
            fanOutQueries: [],
            brandMentioned: null,
            outputTokens: 12,
            webSearch: true,
            webSearchCountryCode: "US",
          },
          {
            status: "error",
            model: "gemini",
            errorCode: "UNSUPPORTED_COUNTRY",
            message: "Gemini doesn’t support country selection.",
          },
        ],
      }),
    );

    const result = await explorePrompt({
      projectId: "p1",
      prompt: "hi",
      models: ["chat_gpt", "gemini"],
      webSearch: true,
      webSearchCountryCode: "US",
    });

    expect(result.results.map((r) => r.status)).toEqual(["success", "error"]);
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/projects/p1/ai-search/prompt-explorer");
    expect(init?.body).toBe(
      JSON.stringify({
        prompt: "hi",
        models: ["chat_gpt", "gemini"],
        webSearch: true,
        webSearchCountryCode: "US",
      }),
    );
  });
});
