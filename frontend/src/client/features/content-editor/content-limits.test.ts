import { describe, expect, it } from "vitest";
import { isArticleContentWithinLimit } from "./content-limits";

describe("isArticleContentWithinLimit", () => {
  it("allows content at the API's byte limit and rejects the next byte", () => {
    expect(isArticleContentWithinLimit("a".repeat(150_000))).toBe(true);
    expect(isArticleContentWithinLimit("a".repeat(150_001))).toBe(false);
  });

  it("counts UTF-8 bytes for Hindi content", () => {
    expect(isArticleContentWithinLimit("ह".repeat(50_001))).toBe(false);
  });
});
