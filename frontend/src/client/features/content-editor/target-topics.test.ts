import { describe, expect, it } from "vitest";
import { parseTargetTopics, primaryTargetTopic } from "./target-topics";

describe("target topics", () => {
  it("splits comma and newline values and removes case-insensitive duplicates", () => {
    expect(parseTargetTopics("SEO, seo\ncontent strategy")).toEqual([
      "SEO",
      "content strategy",
    ]);
    expect(primaryTargetTopic("SEO, content strategy")).toBe("SEO");
  });

  it("limits the list to five topics", () => {
    expect(parseTargetTopics("a,b,c,d,e,f")).toHaveLength(5);
  });
});
