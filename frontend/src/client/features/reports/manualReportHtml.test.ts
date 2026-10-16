import { describe, expect, it } from "vitest";
import { buildManualReportHtml } from "./manualReportHtml";

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
