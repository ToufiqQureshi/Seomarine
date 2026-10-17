import { describe, expect, it } from "vitest";
import {
  searchPreviewTitle,
  searchPreviewUrl,
  titleForDraft,
} from "./editor-display";

const draft = {
  title: "",
  targetKeyword: "",
  slug: "draft",
  metaDescription: "",
  content: "# Article H1\n\nBody",
};

describe("editor title and search preview", () => {
  it("uses the H1 while the page title is empty and only says Untitled when both are empty", () => {
    expect(titleForDraft(draft)).toBe("Article H1");
    expect(searchPreviewTitle(draft)).toBe("Article H1");
    expect(titleForDraft({ ...draft, content: "Body" })).toBe("Untitled draft");
  });

  it("uses the connected workspace domain and slug", () => {
    expect(searchPreviewUrl("revmerito.com", "guide")).toBe(
      "https://revmerito.com/guide",
    );
    expect(searchPreviewUrl("", "guide")).toContain("Connect a website");
  });
});
