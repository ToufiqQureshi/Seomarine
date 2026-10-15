import { MarkdownManager } from "@tiptap/markdown";
import { describe, expect, it } from "vitest";
import { scoreContent } from "../scoring";
import { articleExtensions } from "./extensions";

// Drafts are stored as Markdown and the visual editor rewrites them on every
// change. Anything its schema cannot hold would be silently deleted from a
// customer's article, so every construct the old Markdown field allowed must
// survive a parse and serialize.
const markdown = new MarkdownManager({ extensions: articleExtensions() });
const roundTrip = (source: string) =>
  markdown.serialize(markdown.parse(source));
const squash = (value: string) => value.replace(/\s+/g, " ").trim();

describe("visual editor Markdown round trip", () => {
  it.each([
    ["H1 from old drafts", "# Old title", "# Old title"],
    ["H2", "## What is it?", "## What is it?"],
    ["H3", "### Sub point", "### Sub point"],
    ["H4", "#### Small heading", "#### Small heading"],
    ["bold", "A **strong** word", "**strong**"],
    ["italic", "An *italic* word", "*italic*"],
    ["strikethrough", "An ~~old~~ price", "~~old~~"],
    ["inline code", "Run `npm test` now", "`npm test`"],
    [
      "external link",
      "See [STR](https://str.com/glossary).",
      "[STR](https://str.com/glossary)",
    ],
    [
      "internal link",
      "Read [our services](/services).",
      "[our services](/services)",
    ],
    ["bullet list", "- one\n- two", "- one\n- two"],
    ["numbered list", "1. first\n2. second", "1. first\n2. second"],
    ["quote", "> Fix crawling first.", "> Fix crawling first."],
    [
      "image with alt text",
      "![Chart of RevPAR](https://cdn.example.org/a.webp)",
      "![Chart of RevPAR](https://cdn.example.org/a.webp)",
    ],
    ["Hindi text", "होटल बुकिंग **तेज़**", "होटल बुकिंग **तेज़**"],
  ])("keeps %s", (_, source, expected) => {
    expect(roundTrip(source)).toContain(expected);
  });

  it("keeps a table's cells", () => {
    const out = squash(
      roundTrip("| Option | Price |\n| --- | --- |\n| A | 10 |"),
    );
    expect(out).toMatch(/\| Option \| Price \|/);
    expect(out).toMatch(/\| A \| 10 \|/);
  });

  it("is stable: a second round trip changes nothing", () => {
    const source = [
      "Intro with **bold** and [a link](https://example.org).",
      "## Section",
      "- a\n- b",
      "![Alt text here](https://cdn.example.org/b.webp)",
      "| A | B |\n| --- | --- |\n| 1 | 2 |",
    ].join("\n\n");
    const once = roundTrip(source);
    expect(roundTrip(once)).toBe(once);
  });

  it("does not change the article's score", () => {
    const source = [
      "Hotel revenue management is the practice of selling the right room to the right guest at the right price. It helps independent hotels earn more from the same rooms every night of the year.",
      "## What is hotel revenue management?",
      "It covers pricing, distribution and demand. See [STR](https://str.com/glossary) for the metrics.",
      "- Occupancy\n- ADR\n- RevPAR",
      "![Chart of RevPAR rising](https://cdn.example.org/a.webp)",
      "## FAQ",
      "### Do small hotels need it?",
      "Yes. Even a twenty room property loses money when it sells a festival night at a weekday price.",
    ].join("\n\n");
    const draft = {
      title: "Hotel revenue management guide",
      targetKeyword: "hotel revenue management",
      slug: "hotel-revenue-management",
      metaDescription: "",
    };
    for (const mode of ["seo", "aeo"] as const) {
      expect(
        scoreContent(mode, { ...draft, content: roundTrip(source) }).score,
      ).toBe(scoreContent(mode, { ...draft, content: source }).score);
    }
  });

  it("keeps raw HTML as escaped text, never as a tag", () => {
    const out = roundTrip(
      'Hello <script>alert(1)</script> <img src=x onerror="alert(1)">',
    );
    expect(out).not.toMatch(/<script|<img/i);
  });
});
