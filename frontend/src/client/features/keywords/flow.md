# Keyword research: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. User enters one or more seed terms, selects mode, market, location and result settings.
2. Query/mutation hooks validate UI state and call `keywordsApi.ts`.
3. Go `keywords` resolves the market/location, validates limits and plan access, then requests the necessary provider task(s).
4. Returned rows are normalized, grouped and displayed; save actions persist selected rows for the Saved Keywords feature.

## Implementation map

- `keywordsApi.ts`
- `hooks/useKeywordResearchData.ts`
- `hooks/useKeywordSerpAnalysis.ts`
- `backend/internal/keywords/research.go`
- `backend/internal/keywords/locations.go`

## API and service dependencies

Go package: `backend/internal/keywords`; market normalization is shared with `backend/internal/platform/market`.

## Cost, security and failure boundaries

- Research and SERP calls can be billed by the provider; reject invalid requests before spend and do not add blind POST retries.
- Cache keys must include every option that changes results, project and organization scope.
- Show absent/null provider metrics as unavailable, not as zero.
- Location results must use canonical provider locations; do not silently fall back to a different geography.
