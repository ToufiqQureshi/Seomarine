import { describe, expect, it } from "vitest";
import { insertImageContent } from "./formatting";

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
