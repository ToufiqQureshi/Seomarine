import { describe, expect, it } from "vitest";
import {
  scoreContent,
  type ContentBrief,
  type ContentDraftInput,
} from "./scoring";

const goodText = `# Technical SEO guide

Technical SEO is the process of making websites easy for search engines to crawl and understand. This practical guide explains the main steps for improving technical SEO, from crawl access to indexation, with clear advice for site owners. It also explains how to validate each change and avoid disrupting useful pages.

## What is technical SEO?
Technical SEO means improving the technical foundations that help search engines discover, render and index useful pages. A strong process starts by checking access, then prioritizes problems that affect visitors and search engines. Google Search Central documents crawling and indexing behavior for site owners.

## How do I improve crawlability?
Start by reviewing crawl access and internal links. Keep important pages reachable, use clear navigation, and fix broken routes that block users. These steps help search engines find the pages that matter.

## How do I check indexing?
Compare indexed pages with the pages you intend to publish. Review canonical URLs, robots directives, and sitemap entries. Google Search Console can show indexing details and page issues. Record what changed so your team can verify the result.

## FAQ
### How often should technical SEO be reviewed?
Review it after large site changes and on a regular schedule. Updated in 2026.

Crawlability and indexing are central technical SEO tasks. Review crawlability, indexing, internal links and search engine access during each technical SEO review.\n\n- Check crawlability and indexing\n- Review internal links\n\nRead the [Google Search Central documentation](https://developers.google.com/search/docs) and use examples from your own site. WordPress teams can also document their CMS settings for future editors.`;
const draft: ContentDraftInput = {
  title: "Technical SEO guide for site owners",
  targetKeyword: "technical SEO",
  slug: "technical-seo-guide",
  metaDescription:
    "A practical guide to technical SEO, crawlability and indexing for site owners.",
  content: goodText,
};
const countWordsForTest = (value: string) =>
  value.match(/[\p{L}\p{N}]+/gu)?.length ?? 0;
const brief: ContentBrief = {
  collectedAt: "2026-10-01T00:00:00Z",
  terms: [
    { term: "technical SEO", min: 2, max: 12, domains: 10 },
    { term: "crawlability", min: 1, max: 8, domains: 8 },
    { term: "indexing", min: 1, max: 8, domains: 6 },
    { term: "search engines", min: 1, max: 8, domains: 5 },
    { term: "internal links", min: 1, max: 8, domains: 4 },
  ],
  subtopics: ["crawlability", "indexing"],
  questions: ["How do I improve crawlability?", "How do I check indexing?"],
  competitorTexts: [],
};
const fixtures: Array<{
  name: string;
  draft: ContentDraftInput;
  brief?: ContentBrief;
}> = [
  { name: "good", draft, brief },
  {
    name: "thin",
    draft: { ...draft, content: "## Start\nA short note about technical SEO." },
    brief,
  },
  {
    name: "stuffed",
    draft: { ...draft, content: "technical SEO ".repeat(100) + goodText },
    brief,
  },
  {
    name: "copied",
    draft: { ...draft, content: goodText },
    brief: { ...brief, competitorTexts: [goodText] },
  },
  {
    name: "Hinglish",
    draft: {
      ...draft,
      content:
        "Technical SEO se website ko search engines ke liye samajhna aasan hota hai. Is guide mein hum crawlability aur indexing ko simple steps mein dekhte hain.\n\n## Technical SEO kya hai?\nTechnical SEO website ke technical setup ko improve karne ka process hai. Isse pages crawl aur index karna aasaan hota hai.",
    },
    brief,
  },
  {
    name: "empty",
    draft: {
      title: "",
      targetKeyword: "",
      slug: "",
      metaDescription: "",
      content: "",
    },
  },
  {
    name: "very long",
    draft: {
      ...draft,
      content: (goodText + "\n\n").repeat(1800).slice(0, 150_000),
    },
    brief,
  },
];

describe("content scoring fixtures", () => {
  it.each(fixtures)(
    "scores $name deterministically and handles caps",
    ({ draft: input, brief: inputBrief, name }) => {
      const seo = scoreContent("seo", input, inputBrief);
      expect(seo).toEqual(scoreContent("seo", input, inputBrief));
      expect(seo.score).toBeGreaterThanOrEqual(0);
      expect(seo.score).toBeLessThanOrEqual(100);
      if (name === "good") expect(seo.score).toBeGreaterThanOrEqual(80);
      if (name === "thin") expect(seo.score).toBeLessThanOrEqual(30);
      if (name === "stuffed") {
        expect(seo.score).toBeLessThanOrEqual(40);
        expect(seo.blocksPublish).toBe(true);
      }
      if (name === "copied") expect(seo.score).toBeLessThanOrEqual(30);
      if (name === "Hinglish")
        expect(seo.checks.find((c) => c.id === "readability")?.earned).toBe(7);
      if (name === "empty") expect(seo.score).toBe(0);
      if (name === "very long") expect(input.content.length).toBe(150_000);
    },
  );

  it("runs only brief-independent SEO checks when no brief is supplied", () => {
    const result = scoreContent("seo", draft);
    expect(result.hasBrief).toBe(false);
    expect(result.checks.map((c) => c.id)).not.toContain("term-coverage");
    expect(result.checks.map((c) => c.id)).not.toContain("depth");
    expect(result.checks.map((c) => c.id)).toContain("alt-text");
  });

  it("does not apply English readability limits to long Hinglish passages", () => {
    const longHinglish =
      "Technical SEO se website ko samajhna aasan hota hai, aur is guide mein hum crawlability ke steps dekhte hain. ".repeat(
        35,
      );
    const result = scoreContent("seo", { ...draft, content: longHinglish });
    expect(
      result.checks.find((item) => item.id === "readability")?.earned,
    ).toBe(7);
  });

  it("caps a 400-word repeated-topic draft without a brief and blocks publishing", () => {
    const content = "seo tools ".repeat(120) + "ordinary ".repeat(160);
    const result = scoreContent("seo", {
      ...draft,
      targetKeyword: "seo tools",
      content,
    });
    expect(content.trim().split(/\s+/u)).toHaveLength(400);
    expect(result.score).toBeLessThanOrEqual(40);
    expect(result.cappedAt).toBe(40);
    expect(result.blocksPublish).toBe(true);
  });

  it.each([
    ["below minimum", 1, 0.5],
    ["at minimum", 2, 1],
    ["within range", 5, 1],
    ["above maximum", 13, 1],
  ])("scores brief term coverage correctly %s", (_name, uses, expected) => {
    const termBrief: ContentBrief = {
      ...brief,
      terms: [{ term: "crawlability", min: 2, max: 5, domains: 1 }],
    };
    const result = scoreContent(
      "seo",
      {
        ...draft,
        content: `crawlability `.repeat(uses),
      },
      termBrief,
    );
    expect(
      result.checks.find((item) => item.id === "term-coverage")?.earned,
    ).toBe(30 * expected);
    expect(
      result.checks.find((item) => item.id === "term-ranges")?.earned,
    ).toBe(uses > 5 ? 0 : 10);
  });

  it("applies stuffing and short-draft caps to AEO too", () => {
    const aeoText = `SEO tools are software that help teams audit websites. This concise answer explains practical uses, compares useful checks, and gives readers context for choosing an approach. Teams can review changes carefully and validate outcomes before publishing.

## How do SEO tools help?
SEO tools help teams find issues, group priorities, monitor technical changes, and explain important fixes to colleagues using clear evidence.

- Compare useful reports

For example, Google Search Central provides public guidance. Read the [official documentation](https://developers.google.com/search/docs).

## FAQ
Updated in 2026.`;
    const short = scoreContent("aeo", {
      ...draft,
      targetKeyword: "SEO tools",
      content: aeoText,
    });
    const repeatedTopic = "SEO tools ".repeat(120);
    const filler = "ordinary ".repeat(
      400 - countWordsForTest(aeoText) - countWordsForTest(repeatedTopic),
    );
    const content = aeoText + "\n\n" + repeatedTopic + filler;
    const stuffed = scoreContent("aeo", {
      ...draft,
      targetKeyword: "SEO tools",
      content,
    });
    expect(countWordsForTest(aeoText)).toBeLessThan(100);
    expect(short.score).toBe(30);
    expect(short.cappedAt).toBe(30);
    expect(countWordsForTest(content)).toBe(400);
    expect(stuffed.score).toBe(40);
    expect(stuffed.cappedAt).toBe(40);
    expect(stuffed.blocksPublish).toBe(true);
  });

  it("does not count sentence-initial capitalized words as AEO entities", () => {
    const result = scoreContent("aeo", {
      ...draft,
      content:
        "This guide explains useful ideas. It helps readers understand them.",
    });
    expect(result.checks.find((item) => item.id === "entities")?.earned).toBe(
      0,
    );
  });

  it("does not award image alt-text points when the draft has no images", () => {
    const result = scoreContent("seo", draft);
    expect(result.checks.find((item) => item.id === "alt-text")?.earned).toBe(
      0,
    );
  });

  it("scores AEO dimensions independently and keeps the approved 100-point weights", () => {
    const result = scoreContent("aeo", draft, brief);
    expect(result.checks.map((c) => c.id)).toEqual([
      "direct-answer",
      "definition",
      "question-coverage",
      "list-table",
      "sources",
      "faq",
      "entities",
      "freshness",
    ]);
    expect(result.checks.reduce((sum, c) => sum + c.weight, 0)).toBe(100);
    expect(
      result.checks.find((c) => c.id === "question-coverage")?.earned,
    ).toBe(25);
  });

  it("separates same-site links from external sources", () => {
    const internalPath = scoreContent(
      "seo",
      {
        ...draft,
        content: draft.content + "\n\n[Related page](/related-page)",
      },
      brief,
    );
    const ownHost = scoreContent(
      "seo",
      {
        ...draft,
        content: draft.content + "\n\n[Home](https://www.uplumo.com/home)",
      },
      { ...brief, siteDomain: "uplumo.com" },
    );
    expect(
      internalPath.checks.find((item) => item.id === "sources-internal")
        ?.earned,
    ).toBe(8);
    expect(
      ownHost.checks.find((item) => item.id === "sources-internal")?.earned,
    ).toBe(8);
    expect(
      scoreContent("aeo", draft, {
        ...brief,
        siteDomain: "uplumo.com",
      }).checks.find((item) => item.id === "sources")?.earned,
    ).toBe(15);
  });

  it("requires an exact H2 placement instead of counting the H1 twice", () => {
    const contentWithoutTopicH2 = draft.content.replace(
      /^##[^\n]*technical SEO[^\n]*$/gimu,
      "## General guidance",
    );
    const placement = scoreContent(
      "seo",
      { ...draft, content: contentWithoutTopicH2 },
      brief,
    ).checks.find((item) => item.id === "topic-placement");
    expect(placement?.earned).toBe(11.25);
  });

  it("preserves Unicode phrase boundaries and does not score title or meta length", () => {
    const result = scoreContent("seo", {
      ...draft,
      targetKeyword: "SEO",
      title: "Seoul travel",
      content: "Seoul travel guide for visitors.",
    });
    expect(result.checks.find((c) => c.id === "topic-placement")?.earned).toBe(
      0,
    );
    expect(result.checks.map((c) => c.id)).not.toContain("title-length");
    expect(result.checks.map((c) => c.id)).not.toContain("meta-length");
  });

  it("keeps each SEO score cap and publish block tied to its trigger", () => {
    const stuffed = scoreContent("seo", fixtures[2].draft, brief);
    const copiedBrief = { ...brief, competitorTexts: [goodText] };
    const copied = scoreContent("seo", draft, copiedBrief);
    const short = scoreContent(
      "seo",
      { ...draft, content: "Technical SEO is useful." },
      brief,
    );
    expect(stuffed.cappedAt).toBe(40);
    expect(stuffed.blocksPublish).toBe(true);
    expect(copied.cappedAt).toBe(30);
    expect(copied.blocksPublish).toBe(false);
    expect(short.cappedAt).toBe(30);
    expect(short.blocksPublish).toBe(false);
  });
});
