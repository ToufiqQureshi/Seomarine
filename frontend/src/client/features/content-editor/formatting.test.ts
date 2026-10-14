import { describe, expect, it } from "vitest";
import { formatArticleContent, insertImageContent } from "./formatting";

describe("article formatting toolbar", () => {
  it.each([
    ["heading", "## text"],
    ["heading3", "### text"],
    ["list", "- text"],
    ["numbered-list", "1. text"],
    ["quote", "> text"],
  ] as const)(
    "inserts the %s marker at the current line",
    (format, expected) => {
      expect(formatArticleContent("text", 0, 4, format).content).toBe(expected);
    },
  );

  it("wraps text and inserts a table with selected text", () => {
    expect(formatArticleContent("words", 0, 5, "bold").content).toBe(
      "**words**",
    );
    expect(formatArticleContent("words", 0, 5, "italic").content).toBe(
      "*words*",
    );
    expect(formatArticleContent("Topic", 0, 5, "table").content).toBe(
      "| Column 1 | Column 2 |\n| --- | --- |\n| Topic | Value |",
    );
  });
});

describe("image insertion", () => {
  it("adds an image with descriptive alt text and an encoded URL", () => {
    const result = insertImageContent(
      "Intro\n\nConclusion",
      7,
      7,
      "https://cdn.example.com/article image.jpg",
      "A writer reviewing a draft",
    );

    expect(result.content).toBe(
      "Intro\n\n![A writer reviewing a draft](https://cdn.example.com/article%20image.jpg)\n\nConclusion",
    );
    expect(result.selectionStart).toBe(result.selectionEnd);
    expect(result.selectionStart).toBe(result.content.indexOf("Conclusion"));
  });

  it("rejects non-web image URLs and missing alt text", () => {
    expect(() =>
      insertImageContent("", 0, 0, "javascript:alert(1)", "Example"),
    ).toThrow("Use an image URL");
    expect(() =>
      insertImageContent("", 0, 0, "https://cdn.example.com/image.jpg", " "),
    ).toThrow("description for accessibility");
  });
});
