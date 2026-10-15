export type RefreshQuery = {
  query: string;
  page: string;
  clicks: number;
  impressions: number;
  position: number;
};

export function buildContentRefreshQueue(rows: RefreshQuery[]) {
  const byPage = new Map<
    string,
    {
      page: string;
      clicks: number;
      impressions: number;
      weightedPosition: number;
      queries: RefreshQuery[];
    }
  >();

  for (const row of rows) {
    if (row.position < 5 || row.position > 20 || row.impressions < 50) continue;
    const page = byPage.get(row.page) ?? {
      page: row.page,
      clicks: 0,
      impressions: 0,
      weightedPosition: 0,
      queries: [],
    };
    page.clicks += row.clicks;
    page.impressions += row.impressions;
    page.weightedPosition += row.position * row.impressions;
    page.queries.push(row);
    byPage.set(row.page, page);
  }

  return [...byPage.values()]
    .map((page) => ({
      ...page,
      averagePosition:
        page.impressions > 0 ? page.weightedPosition / page.impressions : 0,
      queries: page.queries
        .sort((a, b) => b.impressions - a.impressions)
        .slice(0, 3),
    }))
    .sort((a, b) => b.impressions - a.impressions);
}
