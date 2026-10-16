# Local SEO research: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. User selects a project market or enters a canonical coordinate/location and requested search terms.
2. `localSeoApi.ts` sends businesses, profile or rank-grid action.
3. Go `localseo` validates bounds and plan access, then calls the shared keyword/DataForSEO local SERP provider.
4. UI shows business rows or each grid point and its rank/result status.

## Implementation map

- `LocalSEOPage.tsx`
- `localSeoApi.ts`
- `backend/internal/localseo/handler.go`
- `backend/internal/keywords/local_serp.go`

## API and service dependencies

Go package: `backend/internal/localseo`; provider functions live in `backend/internal/keywords`.

## Cost, security and failure boundaries

- Local SERP/business requests are paid; validate location, grid dimensions and result limits before spend.
- Per-point failures must remain visible; do not collapse a partial grid into a false complete result.
- No automatic recurring grid checks are created by opening the page.
