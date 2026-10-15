import type { Editor } from "@tiptap/react";
import { describe, expect, it, vi } from "vitest";
import { freshWithinTwelveMonths, freshnessCheck } from "./scoring-rules";
import { applySuggestion } from "./linkSuggestions";
import { validateAuthorInput } from "./authorsApi";

const NOW = new Date("2026-10-15T00:00:00Z");

describe("freshWithinTwelveMonths", () => {
  it("passes inside the window", () => {
    expect(freshWithinTwelveMonths("2026-10-14", NOW)).toBe(true);
    expect(freshWithinTwelveMonths("2026-09-30", NOW)).toBe(true);
  });

  it("stays fresh through the same calendar day, twelve months later", () => {
    expect(freshWithinTwelveMonths("2025-10-15", NOW)).toBe(true);
  });

  it("expires one day after twelve months", () => {
    expect(freshWithinTwelveMonths("2025-10-14", NOW)).toBe(false);
    expect(freshWithinTwelveMonths("2024-10-01", NOW)).toBe(false);
  });

  it("rejects a malformed date instead of treating it as fresh", () => {
    expect(freshWithinTwelveMonths("not-a-date", NOW)).toBe(false);
  });
});

describe("freshnessCheck", () => {
  it("fails without a review date and keeps its 5-point weight", () => {
    const result = freshnessCheck({
      title: "t",
      targetKeyword: "",
      slug: "",
      metaDescription: "",
      content: "c",
      lastReviewedAt: "",
    });
    expect(result).toMatchObject({ passed: false, weight: 5 });
  });

  it("passes when reviewed today", () => {
    const result = freshnessCheck(
      {
        title: "t",
        targetKeyword: "",
        slug: "",
        metaDescription: "",
        content: "c",
        lastReviewedAt: "2026-10-15",
      },
      NOW,
    );
    expect(result.passed).toBe(true);
    expect(result.weight).toBe(5);
  });
});

describe("applySuggestion", () => {
  const editorFor = (text: string) => {
    const chain = {
      focus: vi.fn(),
      setTextSelection: vi.fn(),
      setLink: vi.fn(),
      run: vi.fn().mockReturnValue(true),
    };
    chain.focus.mockReturnValue(chain);
    chain.setTextSelection.mockReturnValue(chain);
    chain.setLink.mockReturnValue(chain);
    const editor = {
      state: {
        doc: {
          descendants: (
            visit: (
              node: { isText: boolean; text: string },
              position: number,
            ) => void,
          ) => visit({ isText: true, text }, 1),
        },
      },
      chain: () => chain,
    } as unknown as Editor;
    return { editor, chain };
  };

  it("selects the intact anchor and applies one TipTap link command", () => {
    const { editor, chain } = editorFor("Read our SEO guide today.");
    expect(applySuggestion(editor, "SEO guide", "https://site.com/seo")).toBe(
      true,
    );
    expect(chain.setTextSelection).toHaveBeenCalledWith({ from: 10, to: 19 });
    expect(chain.setLink).toHaveBeenCalledWith({
      href: "https://site.com/seo",
    });
    expect(chain.run).toHaveBeenCalledTimes(1);
  });

  it("selects Devanagari anchors at their editor offsets", () => {
    const { editor, chain } = editorFor("यह एक गाइड है");
    expect(applySuggestion(editor, "एक गाइड", "https://site.com/g")).toBe(true);
    expect(chain.setTextSelection).toHaveBeenCalledWith({ from: 4, to: 11 });
  });

  it("refuses stale anchors and unsafe links without changing the editor", () => {
    const { editor, chain } = editorFor("hello");
    expect(applySuggestion(editor, "missing", "https://site.com")).toBe(false);
    expect(applySuggestion(editor, "hello", "javascript:alert(1)")).toBe(false);
    expect(chain.run).not.toHaveBeenCalled();
  });
});

describe("validateAuthorInput", () => {
  it("requires a name", () => {
    expect(validateAuthorInput({ name: "", role: "", bio: "" })).toMatch(
      /name/i,
    );
  });

  it("caps the bio at 300 characters", () => {
    expect(
      validateAuthorInput({ name: "A", role: "", bio: "x".repeat(301) }),
    ).toMatch(/300/i);
  });

  it("allows clean https links", () => {
    expect(
      validateAuthorInput({
        name: "A",
        role: "",
        bio: "",
        photoUrl: "https://site.com/a.png",
        profileUrl: "https://site.com/aman",
      }),
    ).toBeNull();
  });

  it("refuses links without a scheme and links with credentials", () => {
    expect(
      validateAuthorInput({
        name: "A",
        role: "",
        bio: "",
        profileUrl: "site.com/aman",
      }),
    ).toMatch(/https/i);
    expect(
      validateAuthorInput({
        name: "A",
        role: "",
        bio: "",
        profileUrl: "https://u:p@site.com",
      }),
    ).toMatch(/credentials/i);
  });
});
