import { describe, expect, it } from "vitest";
import { writerContext } from "./writer-context";

describe("writer context", () => {
  it("sends the selected text instead of the full article", () => {
    expect(
      writerContext(
        "# Title\n\nA long article",
        "selected sentence",
        "paragraph",
      ),
    ).toBe("selected sentence");
  });

  it("sends only the section around the current block", () => {
    expect(
      writerContext(
        "# Title\n\nIntro\n\n## First\n\nCurrent paragraph\n\n## Second\n\nOther",
        "",
        "Current paragraph",
      ),
    ).toBe("## First\n\nCurrent paragraph");
  });

  it("caps fallback article context", () => {
    expect(writerContext("x".repeat(8_000), "", "")).toHaveLength(5_000);
  });
});
