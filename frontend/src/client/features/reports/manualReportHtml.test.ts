import { describe, expect, it } from "vitest";
import {
  buildManualReportHtml,
  buildManualWorkLogHtml,
} from "./manualReportHtml";

describe("buildManualReportHtml", () => {
  it("escapes user content and preserves paragraphs and line breaks", () => {
    const html = buildManualReportHtml({
      title: ` <script>alert("x")</script> `,
      summary: "Clicks & leads",
      body: "First line\nsecond line\n\nNext <section>",
    });

    expect(html).toContain("&lt;script&gt;alert(&quot;x&quot;)&lt;/script&gt;");
    expect(html).toContain("Clicks &amp; leads");
    expect(html).toContain("First line<br>second line");
    expect(html).toContain("<p>Next &lt;section&gt;</p>");
    expect(html).not.toContain("<script>");
    expect(html.endsWith("</html>")).toBe(true);
  });
});

describe("buildManualWorkLogHtml", () => {
  it("escapes work notes and links only safe evidence URLs", () => {
    const html = buildManualWorkLogHtml({
      title: "Client update",
      summary: "Work & results",
      period: "2026-10",
      workCompleted: "Fixed <title> tags",
      outcomes: "No measured change yet",
      evidenceUrls: ["https://example.com/ticket?a=1&b=2"],
      nextSteps: "Review <script>alert(1)</script>",
    });
    expect(html).toContain("Fixed &lt;title&gt; tags");
    expect(html).toContain('href="https://example.com/ticket?a=1&amp;b=2"');
    expect(html).toContain("Review &lt;script&gt;alert(1)&lt;/script&gt;");
    expect(html).not.toContain("<script>");
  });

  it("rejects non-http evidence URLs and credentials", () => {
    const base = {
      title: "Client update",
      summary: "Summary",
      period: "2026-10",
      workCompleted: "Work",
      outcomes: "",
      nextSteps: "",
    };
    expect(() =>
      buildManualWorkLogHtml({
        ...base,
        evidenceUrls: ["javascript:alert(1)"],
      }),
    ).toThrow();
    expect(() =>
      buildManualWorkLogHtml({
        ...base,
        evidenceUrls: ["https://user:pass@example.com"],
      }),
    ).toThrow();
  });
});
