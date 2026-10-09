# Google Search Console

## Legacy behavior and edge cases captured before the port

| Behavior | Rule |
|---|---|
| Each user can link several Google grants. | List each grant's sites independently; a revoked grant asks for reconnection, other provider failures mark properties unavailable. |
| A project selects one verified property from an owned grant. | Keep the `siteUrl` byte-for-byte, reject unverified or unavailable properties, preserve email only for the same grant identity. |
| Legacy mappings may have no Google account ID. | Token lookup falls back to that connector user's first grant. |
| Performance accepts date ranges, dimensions, filters, search type and page offset. | Dates are UTC, recent ranges stop three days before today, row limit is 1–1000 and filters must be inside `dimensionFilterGroups`. |
| Search Performance page queries current/previous daily rows, query-page pairs and countries. | Use impression-weighted average position, show striking-distance queries only when their best page ranks 5–20. |
| Queries/pages table is paginated. | Fetch one extra row for `hasNextPage`; export is capped at 1000. |
| URL inspection handles a list of URLs. | Capture individual URL errors while grant/token errors fail the batch. |

## Status

The Google API transport and GSC provider client are being ported. Routes,
repository, UI contracts and Search Performance shaping are pending.

## Rules

- Google first-party calls are free; they are still quota-limited and use
  bounded retries only for read-only operations.
- A provider 401 triggers one token refresh and one retry.
- Every repository query must include the project organization and user scope.
