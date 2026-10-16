# Search Console performance: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. User selects date window, property, filters and dimension tabs.
2. `searchPerformanceQueries.ts` keys requests by the full filter set and `searchPerformanceApi.ts` calls Go GSC routes.
3. Go verifies project access and selected property, validates Google API filters and executes a bounded read.
4. UI renders rows, totals and pagination; query changes do not mutate GSC data.

## Implementation map

- `SearchPerformancePage.tsx`
- `searchPerformanceQueries.ts`
- `searchPerformanceApi.ts`
- `backend/internal/gsc/performance.go`

## API and service dependencies

Go package: `backend/internal/gsc`; tokens and selected-property ownership are managed by `backend/internal/google` and `gsc`.

## Content refresh queue

The queue reuses the current Search Console report rows; it does not make another Google request. From the report's 100 highest-impression striking-distance query rows, `contentRefreshQueue.ts` keeps queries at average position 5–20 with at least 50 impressions, groups them by page, sums clicks/impressions and computes impression-weighted position. It shows the three strongest query themes per page and the first 25 candidate pages. Review links are accepted only for safe HTTP(S) URLs. These are investigation candidates, not proof that content is stale; the user decides whether to update a page.

## Cost, security and failure boundaries

- Search Console has provider quotas; preserve bounded read retries and avoid redundant refetches.
- Every request uses the project’s selected property and OAuth grant; never accept a property from another project.
- Google returns sampled/limited rows; preserve response metadata and do not claim exhaustive totals.
