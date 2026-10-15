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
  // Reviewed recently: the freshness rule reads the review date, not the
  // article text.
  lastReviewedAt: "2026-10-01",
};
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

describe("scoring rule mutation checks", () => {
  it.each([
    [
      "term-coverage",
      (value: ContentDraftInput) => ({
        ...value,
        content: value.content.replace(/crawlability/giu, "site access"),
      }),
    ],
    [
      "term-ranges",
      (value: ContentDraftInput) => ({
        ...value,
        content: value.content + " " + "technical SEO ".repeat(40),
      }),
    ],
    [
      "topic-placement",
      (value: ContentDraftInput) => ({
        ...value,
        title: "A guide for site owners",
        content: value.content.replace(/technical SEO/giu, "site optimization"),
      }),
    ],
    [
      "headings",
      (value: ContentDraftInput) => ({
        ...value,
        content: value.content.replace(/^#{1,6} .+$/gmu, ""),
      }),
    ],
    [
      "depth",
      (value: ContentDraftInput) => ({
        ...value,
        content: "Short useful note.",
      }),
    ],
    [
      "subtopics",
      (value: ContentDraftInput) => ({
        ...value,
        content: value.content.replace(
          /crawlability|indexing/giu,
          "site setup",
        ),
      }),
    ],
    [
      "list-table",
      (value: ContentDraftInput) => ({
        ...value,
        content: value.content.replace(/^[-*+] .+$/gmu, ""),
      }),
    ],
    [
      "sources-internal",
      (value: ContentDraftInput) => ({
        ...value,
        content: value.content.replace(/\[[^\]]+\]\([^)]+\)/gu, "source"),
      }),
    ],
    [
      "readability",
      (value: ContentDraftInput) => ({
        ...value,
        content: value.content.replace(/\n\n/gu, " "),
      }),
    ],
    [
      "alt-text",
      (value: ContentDraftInput) => ({
        ...value,
        content: value.content.replace(
          "A detailed illustration of a technical SEO audit",
          "x",
        ),
      }),
    ],
  ])("SEO rule %s responds to a targeted input mutation", (id, mutate) => {
    const imageReady = {
      ...draft,
      content:
        draft.content +
        "\n\n![A detailed illustration of a technical SEO audit](https://images.example.org/seo.png)",
    };
    const scoreBrief = id === "headings" ? undefined : brief;
    const baseline = scoreContent("seo", imageReady, scoreBrief).checks.find(
      (item) => item.id === id,
    );
    const changed = scoreContent(
      "seo",
      mutate(imageReady),
      scoreBrief,
    ).checks.find((item) => item.id === id);
    expect(baseline?.earned, id).toBeGreaterThan(0);
    expect(changed?.earned, id).toBeLessThan(baseline?.earned ?? 0);
  });

  it.each([
    [
      "direct-answer",
      (value: ContentDraftInput) => ({
        ...value,
        content: value.content.replace(
          /^Technical SEO is[^\n]+/mu,
          "Technical SEO helps.",
        ),
      }),
    ],
    [
      "definition",
      (value: ContentDraftInput) => ({
        ...value,
        content: value.content.replace(
          /Technical SEO (?:is|means|are|refers to|is defined as)/giu,
          "Technical SEO stays",
        ),
      }),
    ],
    [
      "question-coverage",
      (value: ContentDraftInput) => ({
        ...value,
        content: value.content.replace(
          /^## How do I improve crawlability\?[\s\S]*?(?=^## |^Read the)/gmu,
          "",
        ),
      }),
    ],
    [
      "list-table",
      (value: ContentDraftInput) => ({
        ...value,
        content: value.content.replace(/^[-*+] .+$/gmu, ""),
      }),
    ],
    [
      "sources",
      (value: ContentDraftInput) => ({
        ...value,
        content: value.content.replace(
          /\[[^\]]+\]\([^)]+\)/gu,
          "documentation",
        ),
      }),
    ],
    [
      "faq",
      (value: ContentDraftInput) => ({
        ...value,
        content: value.content.replace(/^## FAQ\n[\s\S]*?(?=^Read the)/gmu, ""),
      }),
    ],
    [
      "entities",
      (value: ContentDraftInput) => ({
        ...value,
        content: value.content.toLocaleLowerCase(),
      }),
    ],
    [
      "freshness",
      (value: ContentDraftInput) => ({
        ...value,
        lastReviewedAt: "2024-01-01",
      }),
    ],
  ])("AEO rule %s responds to a targeted input mutation", (id, mutate) => {
    const baseline = scoreContent("aeo", draft, brief).checks.find(
      (item) => item.id === id,
    );
    const changed = scoreContent("aeo", mutate(draft), brief).checks.find(
      (item) => item.id === id,
    );
    expect(baseline?.earned, id).toBeGreaterThan(0);
    expect(changed?.earned, id).toBeLessThan(baseline?.earned ?? 0);
  });
});
