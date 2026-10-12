import { describe, expect, it } from "vitest";
import type { LighthouseIssue } from "./types";
import { categoryLabel, issuesToTable } from "./utils";

const issue: LighthouseIssue = {
  category: "performance",
  auditKey: "largest-contentful-paint",
  title: "Improve image delivery",
  description: "Serve smaller images.",
  score: 0.5,
  scoreDisplayMode: "numeric",
  displayValue: "2.5 s",
  impactMs: 320,
  impactBytes: null,
  severity: "warning",
  items: ["/hero.png", "/banner.png"],
};

describe("Lighthouse issue export helpers", () => {
  it("keeps CSV headers and row cells aligned, including nullable values", () => {
    const table = issuesToTable([issue]);
    expect(table.headers).toHaveLength(9);
    expect(table.rows).toEqual([
      [
        "performance",
        "warning",
        0.5,
        "Improve image delivery",
        "2.5 s",
        "Serve smaller images.",
        320,
        "",
        2,
      ],
    ]);
    expect(issuesToTable([]).rows).toEqual([]);
  });

  it("uses readable labels for special and regular categories", () => {
    expect(categoryLabel("best-practices")).toBe("Best practices");
    expect(categoryLabel("all")).toBe("All");
    expect(categoryLabel("performance")).toBe("Performance");
  });
});
