import { describe, expect, it } from "vitest";
import { normalizeLinkUrl } from "./link-url";

describe("normalizeLinkUrl", () => {
  it.each([
    ["example.com", "https://example.com"],
    ["  str.com/glossary  ", "https://str.com/glossary"],
    ["https://example.com/a?b=1#c", "https://example.com/a?b=1#c"],
    ["http://example.com", "http://example.com"],
    ["/services/revenue-management", "/services/revenue-management"],
    ["#faq", "#faq"],
    ["mailto:hello@example.com", "mailto:hello@example.com"],
    ["होटल.भारत", "https://होटल.भारत"],
  ])("accepts %s", (input, expected) => {
    expect(normalizeLinkUrl(input)).toBe(expected);
  });

  it.each([
    "",
    "   ",
    "javascript:alert(1)",
    "JavaScript:alert(1)",
    "data:text/html,hi",
    "vbscript:msgbox",
    "//evil.example",
    "https://user:pass@example.com",
    "two words",
    "localhost",
    "mailto:not-an-address",
  ])("refuses %j", (input) => {
    expect(normalizeLinkUrl(input)).toBeNull();
  });
});
