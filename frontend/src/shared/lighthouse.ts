export const LIGHTHOUSE_CATEGORIES = [
  "performance",
  "accessibility",
  "best-practices",
  "seo",
] as const;

export const LIGHTHOUSE_CATEGORY_TABS = [
  "all",
  ...LIGHTHOUSE_CATEGORIES,
] as const;

export type LighthouseCategoryTab = (typeof LIGHTHOUSE_CATEGORY_TABS)[number];
