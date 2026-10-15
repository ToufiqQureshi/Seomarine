import { describe, expect, it } from "vitest";
import {
  briefForWriter,
  briefMatchesTopic,
  termUsage,
  toScoringBrief,
} from "./brief-view";
import type { SearchBrief } from "./contentEditorApi";
import { scoreContent } from "./scoring";

const brief: SearchBrief = {
  keyword: "Hotel Revenue Management",
  locationCode: 2356,
  languageCode: "en",
  fetchedAt: "2026-10-15T10:00:00Z",
  questions: ["What is RevPAR?"],
  competitors: [
    {
      url: "https://a.test/guide",
      title: "Guide",
      headings: ["What is RevPAR?"],
      wordCount: 900,
      text: "Channel manager and RevPAR explained.",
    },
  ],
  terms: [
    { term: "channel manager", min: 2, max: 4, domains: 6 },
    { term: "revpar", min: 1, max: 2, domains: 5 },
  ],
  subtopics: ["revpar"],
};

describe("brief view", () => {
  it("applies only to the topic it was built for", () => {
    expect(briefMatchesTopic(brief, "  hotel   revenue management ")).toBe(
      true,
    );
    expect(briefMatchesTopic(brief, "hotel booking engine")).toBe(false);
  });

  it("marks each term under, in range or over", () => {
    const content =
      "A channel manager helps. RevPAR, RevPAR and RevPAR again. Channel manager.";
    expect(
      termUsage(brief, content).map(({ term, used, status }) => [
        term,
        used,
        status,
      ]),
    ).toEqual([
      ["channel manager", 2, "in-range"],
      ["revpar", 3, "over"],
    ]);
    expect(termUsage(brief, "")[0]?.status).toBe("under");
  });

  it("feeds the scoring engine, which then scores brief checks", () => {
    const draft = {
      title: "Hotel revenue management",
      targetKeyword: "hotel revenue management",
      slug: "x",
      metaDescription: "",
      content: "Short text about channel manager.",
    };
    const ids = (withBrief: boolean) =>
      scoreContent(
        "seo",
        draft,
        withBrief ? toScoringBrief(brief) : undefined,
      ).checks.map((check) => check.id);
    expect(ids(true)).toContain("term-coverage");
    expect(ids(false)).not.toContain("term-coverage");
  });

  it("gives the writer terms, questions and headings but no page text", () => {
    const text = briefForWriter(brief);
    expect(text).toContain("- channel manager: 2-4");
    expect(text).toContain("- What is RevPAR?");
    expect(text).not.toContain("explained");
    expect(briefForWriter(null)).toBe("");
  });

  it("keeps the writer brief bounded", () => {
    const big = {
      ...brief,
      competitors: Array.from({ length: 8 }, () => ({
        ...brief.competitors[0],
        headings: Array.from({ length: 50 }, () => "x".repeat(200)),
      })),
    };
    expect(briefForWriter(big).length).toBeLessThanOrEqual(6_000);
  });
});
