import { describe, expect, it } from "vitest";
import { withBrandBar } from "@/server/features/branding/brandBar";
import type { Branding } from "@/types/schemas/branding";

const branding: Branding = {
  brandName: `Acme <script>alert(1)</script>`,
  accentColor: "#ff0000",
  logoDataUrl: null,
  websiteUrl: "https://acme.agency/",
  updatedAt: "2026-10-06T00:00:00.000Z",
};

describe("withBrandBar", () => {
  it("puts the escaped bar right after the opening body tag", () => {
    const html = withBrandBar(
      `<!doctype html><html><body class="r"><h1>Report</h1></body></html>`,
      branding,
    );
    expect(html).toMatch(
      /^<!doctype html><html><body class="r"><div data-openseo-brand-bar/,
    );
    expect(html).toContain("Acme &lt;script&gt;");
    expect(html).not.toContain("<script>");
    expect(html).toContain(">acme.agency</a>");
  });

  it("leaves a document without a body tag untouched", () => {
    const html = "<html><h1>Report</h1></html>";
    expect(withBrandBar(html, branding)).toBe(html);
  });
});
