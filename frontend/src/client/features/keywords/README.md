# Keyword research and saved keywords (frontend)

## What this does

Pages `/p/$projectId/keywords` (research) and `/p/$projectId/saved` (saved
keywords), plus the search-performance and domain-overview "save keyword"
actions. All data comes from the Go keywords API.

## Flow

- Research: the controls form builds a request (`keywords`, optional
  `locationCode`/`locationName`, `resultLimit`, `mode`, `clickstream`,
  `groupKeywords`) and the query cache keys on every field that changes the
  result, including the local area. Every research call is billed, so queries
  never auto-retry.
- SERP panel: a shallow top-20 snapshot by default; paging past it re-buys a
  depth-100 snapshot one level deeper than the current one. A failed deep
  fetch drops back to the cached shallow snapshot instead of re-billing on
  every retry.
- Saved keywords: filtered/sorted/paged list (page sizes 50/100/250), bulk
  remove (max 2000 ids), tag assign (append/replace), tag rename/recolor, and
  a delete that fails with `tag_in_use` while any keyword still carries the
  tag. Refresh re-buys metrics for every saved keyword.
- Export CSV/Sheets: runs the same list filters through `saved/export` and
  hands the rows to the shared exporter.

## Go endpoints

`POST /api/v1/projects/{projectId}/keywords/...` — `research`, `serp`,
`saved/refresh|save|list|export|remove`, `saved/tags/assign|update|delete`.
Contract: `backend/api/keywords.yaml`; response shapes in
`keywordsApi.ts` (Zod) mirror `backend/internal/keywords`.

## Rules that must never break

- Response validation through `apiRequest` + Zod: a drifted server shape is an
  "unexpected answer" error, never a half-rendered table.
- Never sneak a billed refetch past the user (retries off for research/serp;
  refresh and SERP depth changes are explicit).
- Playwright runs research against `VITE_E2E_KEYWORD_FIXTURES` canned rows,
  never against DataForSEO.
- The fixture use for e2e and tests stays opt-in; production builds call the
  API.
