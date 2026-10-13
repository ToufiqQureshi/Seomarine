import { describe, expect, it } from "vitest";
import type { SavedKeywordRow } from "@/types/keywords";
import {
  formatSavedKeywordDate,
  formatSavedKeywordNumber,
  savedKeywordExportRow,
  toSavedKeywordSort,
} from "./savedKeywordsUtils";

const row: SavedKeywordRow = {
  id: "saved-1",
  projectId: "project-1",
  keyword: "best dentist",
  locationCode: 2840,
  languageCode: "en",
  createdAt: "2026-10-12T00:00:00Z",
  searchVolume: 1200,
  cpc: null,
  competition: 0.4,
  keywordDifficulty: 31,
  intent: "commercial",
  monthlySearches: [],
  fetchedAt: "2026-10-11T00:00:00Z",
  tags: [
    { id: "tag-1", name: "Local", normalizedName: "local", color: "blue" },
    { id: "tag-2", name: "Priority", normalizedName: "priority", color: null },
  ],
};

describe("saved keyword utilities", () => {
  it("keeps CSV columns aligned and represents missing metrics as empty cells", () => {
    expect(savedKeywordExportRow(row)).toEqual([
      "best dentist",
      1200,
      "",
      0.4,
      31,
      "commercial",
      "Local, Priority",
      "2026-10-11T00:00:00Z",
    ]);
    expect(
      savedKeywordExportRow({
        ...row,
        searchVolume: null,
        tags: [],
        fetchedAt: null,
      }),
    ).toEqual(["best dentist", "", "", 0.4, 31, "commercial", "", ""]);
  });

  it("accepts only supported sorts and falls back to newest first", () => {
    for (const sort of [
      "keyword",
      "searchVolume",
      "cpc",
      "competition",
      "keywordDifficulty",
      "fetchedAt",
    ]) {
      expect(toSavedKeywordSort(sort)).toBe(sort);
    }
    expect(toSavedKeywordSort(undefined)).toBe("createdAt");
    expect(toSavedKeywordSort("createdBy" as string)).toBe("createdAt");
  });

  it("formats null metrics and dates without throwing", () => {
    expect(formatSavedKeywordNumber(null)).toBe("-");
    expect(formatSavedKeywordNumber(1200)).toBe(
      new Intl.NumberFormat().format(1200),
    );
    expect(formatSavedKeywordDate(null)).toBe("-");
    expect(formatSavedKeywordDate("2026-10-11T00:00:00Z")).toBe(
      new Date("2026-10-11T00:00:00Z").toLocaleDateString(),
    );
  });
});
