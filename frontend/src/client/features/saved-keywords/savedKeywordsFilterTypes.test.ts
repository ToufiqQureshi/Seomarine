import { describe, expect, it } from "vitest";
import {
  EMPTY_SAVED_KEYWORDS_FILTERS,
  compileSavedKeywordsFilters,
  countActiveSavedKeywordsFilters,
} from "./savedKeywordsFilterTypes";

describe("saved keyword filters", () => {
  it("splits and normalizes include/exclude terms and numeric bounds", () => {
    expect(
      compileSavedKeywordsFilters({
        ...EMPTY_SAVED_KEYWORDS_FILTERS,
        include: "  dentist, Dental + clinic ",
        exclude: "free, cheap",
        minVol: "12.9",
        maxVol: "900",
        minCpc: "0.25",
        maxCpc: "2.5",
        minKd: "5.8",
        maxKd: "100",
      }),
    ).toEqual({
      includeTerms: ["dentist", "dental", "clinic"],
      excludeTerms: ["free", "cheap"],
      minVolume: 12,
      maxVolume: 900,
      minCpc: 0.25,
      maxCpc: 2.5,
      minDifficulty: 5,
      maxDifficulty: 100,
    });
  });

  it("ignores non-numeric text and clamps difficulty into its domain", () => {
    expect(
      compileSavedKeywordsFilters({
        ...EMPTY_SAVED_KEYWORDS_FILTERS,
        minVol: "NaN",
        minCpc: "Infinity",
        minKd: "-5",
        maxKd: "101",
      }),
    ).toMatchObject({
      includeTerms: undefined,
      excludeTerms: undefined,
      minVolume: undefined,
      minCpc: undefined,
      minDifficulty: 0,
      maxDifficulty: 100,
    });
  });

  it("counts only non-whitespace fields", () => {
    expect(countActiveSavedKeywordsFilters(EMPTY_SAVED_KEYWORDS_FILTERS)).toBe(
      0,
    );
    expect(
      countActiveSavedKeywordsFilters({
        ...EMPTY_SAVED_KEYWORDS_FILTERS,
        include: "  dentist ",
        maxCpc: "1.2",
      }),
    ).toBe(2);
  });
});
