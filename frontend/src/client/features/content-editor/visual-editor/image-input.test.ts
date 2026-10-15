import { MarkdownManager } from "@tiptap/markdown";
import { describe, expect, it } from "vitest";
import { articleExtensions } from "./extensions";
import { pickImageFile, validateImage } from "./image-input";

describe("validateImage", () => {
  it("encodes the URL and keeps the alt text on one line", () => {
    expect(
      validateImage(
        "https://cdn.example.com/article image.jpg",
        "  A writer\nreviewing [a] draft ",
      ),
    ).toEqual({
      src: "https://cdn.example.com/article%20image.jpg",
      alt: "A writer reviewing a draft",
    });
  });

  it.each([
    ["javascript:alert(1)", "Example"],
    ["data:image/png;base64,AAAA", "Example"],
    ["not a url", "Example"],
    ["https://cdn.example.com/a.webp", "   "],
    ["https://cdn.example.com/a.webp", "[]"],
  ])("refuses %s with alt %j", (source, alt) => {
    expect(() => validateImage(source, alt)).toThrow(TypeError);
  });

  it("survives the editor's Markdown round trip unchanged", () => {
    const image = validateImage(
      "https://cdn.example.com/a (1).webp",
      "Chart: RevPAR (festival week)",
    );
    const markdown = new MarkdownManager({ extensions: articleExtensions() });
    const doc = {
      type: "doc",
      content: [{ type: "image", attrs: image }],
    };
    const parsed = markdown.parse(markdown.serialize(doc));
    expect(parsed.content?.[0]?.attrs).toMatchObject(image);
  });
});

const file = (name: string, type: string) => new File(["x"], name, { type });

describe("pickImageFile", () => {
  it("takes the first PNG or JPEG and ignores other files", () => {
    const jpeg = file("photo.jpg", "image/jpeg");
    expect(pickImageFile([file("notes.pdf", "application/pdf"), jpeg])).toBe(
      jpeg,
    );
  });

  it.each([
    [[file("anim.gif", "image/gif")]],
    [[file("vector.svg", "image/svg+xml")]],
    [[]],
    [null],
  ])("returns nothing for %#", (files) => {
    expect(pickImageFile(files)).toBeUndefined();
  });
});
