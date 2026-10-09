# Google Search Console

## Legacy behavior and edge cases captured before the port

| Behavior                                                                                     | Rule                                                                                                                               |
| -------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| Each user can link several Google grants.                                                    | List each grant's sites independently; a revoked grant asks for reconnection, other provider failures mark properties unavailable. |
| A project selects one verified property from an owned grant.                                 | Keep the `siteUrl` byte-for-byte, reject unverified or unavailable properties, preserve email only for the same grant identity.    |
| Legacy mappings may have no Google account ID.                                               | Token lookup falls back to that connector user's first grant.                                                                      |
| Performance accepts date ranges, dimensions, filters, search type and page offset.           | Dates are UTC, recent ranges stop three days before today, row limit is 1–1000 and filters must be inside `dimensionFilterGroups`. |
| Search Performance page queries current/previous daily rows, query-page pairs and countries. | Use impression-weighted average position, show striking-distance queries only when their best page ranks 5–20.                     |
| Queries/pages table is paginated.                                                            | Fetch one extra row for `hasNextPage`; export is capped at 1000.                                                                   |
| URL inspection handles a list of URLs.                                                       | Capture individual URL errors while grant/token errors fail the batch.                                                             |

## Status

The Go backend now serves the Search Performance report, query/page table and
export endpoints. It reads the existing `gsc_connections` mapping and uses the
shared Google API client. GSC consent, property selection, URL inspection, MCP
wrappers and the React call-site switch remain on the TypeScript side.

## Search Performance API

All routes are POST, require a session and project access, and reject unknown
JSON fields. `dateRange` defaults to `last_28_days`; accepted ranges are
`last_7_days`, `last_28_days` and `last_3_months`. The report runs four bounded
Search Analytics reads, preserves the legacy 3-day data lag and 16-month history
floor, and returns impression-weighted totals and best-page striking-distance
rows. Filters use one AND group; country is omitted from the country breakdown.
The table accepts `query` or `page`, pages 1-based at sizes 25/50/100, and uses
one extra row to compute `hasNextPage`. Export is capped at 1000 rows.

Routes and request/response contracts are in `backend/api/gsc.yaml`.

## Deliberate difference

The existing provider client rejects Search Console filter expressions longer
than 1024 UTF-8 bytes before a provider call. The legacy page schema accepts up
to 4096 JavaScript characters, so Go returns a 400 for expressions over the
provider client's safe request bound. This prevents sending a request that the
Go client itself rejects; changing the shared provider bound needs a separate
contract review.

## Rules

- Google first-party calls are free; they are still quota-limited and use
  bounded retries only for read-only operations.
- A provider 401 triggers one token refresh and one retry.
- Every repository query must include the project organization and user scope.
