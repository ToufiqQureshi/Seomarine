import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ArticlePreview } from "./ArticlePreview";

// Tailwind's reset strips the browser's heading and list styles, so a preview
// without per-element classes shows every block as plain text.
describe("ArticlePreview", () => {
  const html = renderToStaticMarkup(
    createElement(ArticlePreview, {
      content:
        "## Section\n\n### Sub\n\n- one\n\n1. first\n\n[source](https://example.org)\n\n[bad](javascript:alert(1))\n\n![a chart](https://cdn.example.org/a.webp)",
    }),
  );

  it.each([
    [
      "an H2 that looks like a heading",
      /<h2 class="[^"]*text-2xl[^"]*font-semibold/,
    ],
    ["an H3 that looks like a heading", /<h3 class="[^"]*text-xl/],
    ["bullets on unordered lists", /<ul class="[^"]*list-disc/],
    ["numbers on ordered lists", /<ol class="[^"]*list-decimal/],
    [
      "styled safe links",
      /<a [^>]*href="https:\/\/example.org"[^>]*class="[^"]*underline/,
    ],
    [
      "images with their alt text",
      /<img src="https:\/\/cdn.example.org\/a.webp" alt="a chart"/,
    ],
  ])("renders %s", (_, pattern) => {
    expect(html).toMatch(pattern);
  });

  it("never renders a javascript: link", () => {
    expect(html).not.toContain("javascript:");
  });
});
