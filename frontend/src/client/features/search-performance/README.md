# Search Console performance

Reads Search Console metrics by date range and dimensions such as query, page, country and device.

Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for the rules that protect user data, provider spend and customer trust.

## Code map

- `SearchPerformancePage.tsx`
- `searchPerformanceQueries.ts`
- `searchPerformanceApi.ts`
- `backend/internal/gsc/performance.go`

- `ContentRefreshQueue.tsx` and `contentRefreshQueue.ts` (read-only candidates from loaded report rows)
