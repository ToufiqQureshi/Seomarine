# Search Console performance: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Reads Search Console metrics by date range and dimensions such as query, page, country and device.

## Decisions to preserve

- Search Console has provider quotas; preserve bounded read retries and avoid redundant refetches. _Why:_ Google limits read volume, so bounded queries and no refetch loops preserve quota for useful reports.
- Every request uses the project’s selected property and OAuth grant; never accept a property from another project. _Why:_ A valid Google grant for one project does not authorize reading another project’s property.
- Google returns sampled/limited rows; preserve response metadata and do not claim exhaustive totals. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.
- Build the refresh queue from the already loaded Search Console rows and keep its position/impression thresholds and page cap explicit. _Why:_ Reusing report data avoids extra quota use and keeps the queue bounded.
- Label the rows as review candidates, not confirmed stale content, and keep content edits separate from this read-only queue. _Why:_ Search metrics identify opportunities but do not prove a page is outdated or authorize a site change.

## Verification pointers

- `searchPerformanceQueries.test.ts`
