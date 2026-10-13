import { describe, expect, it } from "vitest";
import { scoreContent, type ContentDraftInput } from "./scoring";

const completeDraft: ContentDraftInput = {
  title: "Beginner guide to technical SEO: improve your website",
  targetKeyword: "technical SEO",
  slug: "technical-seo-guide",
  metaDescription:
    "Learn technical SEO with a practical guide to crawlability, indexing, page speed, and the checks that help search engines understand your website content.",
  content: `Technical SEO helps search engines discover and understand useful pages. This guide explains the main checks in clear steps for site owners.

## What is technical SEO?
Technical SEO is the work that helps search engines crawl, render, and index a website reliably. It refers to the foundations behind useful content.

## How do I start?
Follow these practical steps to improve your technical SEO:
- Check that important pages can be crawled.
- Keep page titles and canonical URLs clear.
- Review site speed and mobile usability.

## How can I verify the changes?
Compare the crawl and indexing reports after making each change, then monitor results over several weeks. Add useful context and explain any issue that affects users.

## Where can I learn more?
Read the official [Google Search Central guide](https://developers.google.com/search/docs) for current documentation and recommendations. It covers helpful content and technical SEO fundamentals for publishers.

Write for people first. Explain each technical term and show examples. A reliable technical SEO process helps teams identify problems, plan fixes, and verify improvements without promising a specific ranking outcome.`,
};

describe("content editor scoring", () => {
  it("scores SEO and AEO separately with weighted, actionable checks", () => {
    const seo = scoreContent("seo", completeDraft);
    const aeo = scoreContent("aeo", completeDraft);

    expect(seo.score).toBeGreaterThan(60);
    expect(aeo.score).toBeGreaterThan(60);
    expect(seo.checks.map(({ id }) => id)).toContain("topic-body");
    expect(aeo.checks.map(({ id }) => id)).toContain("direct-answer");
    expect(seo.checks.map(({ id }) => id)).not.toContain("direct-answer");
    expect(aeo.checks.map(({ id }) => id)).not.toContain("keyword-coverage");
  });

  it("returns zero for an empty draft and exposes missing setup checks", () => {
    const empty = {
      title: "",
      targetKeyword: "",
      slug: "",
      metaDescription: "",
      content: "",
    };

    expect(scoreContent("seo", empty).score).toBe(0);
    expect(scoreContent("aeo", empty).score).toBe(0);
    expect(
      scoreContent("seo", empty).checks.every((check) => !check.passed),
    ).toBe(true);
  });

  it("does not use fixed word-count or keyword-density targets as ranking rules", () => {
    const checks = scoreContent("seo", completeDraft).checks;

    expect(checks.map(({ id }) => id)).not.toContain("keyword-coverage");
    expect(checks.map(({ id }) => id)).not.toContain("depth");
    expect(checks.map(({ id }) => id)).not.toContain("title-length");
    expect(checks.map(({ id }) => id)).not.toContain("meta-length");
    expect(checks.find(({ id }) => id === "substance")?.detail).toContain(
      "no universal word-count target",
    );
  });

  it("matches whole topic phrases instead of substrings inside other words", () => {
    const nearMatch = {
      ...completeDraft,
      title: "Seoul guide for new visitors and local travellers",
      targetKeyword: "SEO",
      content:
        "Seoul has many useful places and an easy metro system for visitors.",
    };
    const seo = scoreContent("seo", nearMatch);
    expect(seo.checks.find(({ id }) => id === "topic-title")?.passed).toBe(
      false,
    );
    expect(seo.checks.find(({ id }) => id === "topic-body")?.passed).toBe(
      false,
    );
  });
});
