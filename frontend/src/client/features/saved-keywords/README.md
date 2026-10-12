# Saved keywords UI (frontend)

## What this does

Table, filters, tags and bulk actions for `/p/$projectId/saved`. Data comes
from the Go keywords API via `@/client/features/keywords/keywordsApi`
(`keywords/saved/*` routes; contract in `backend/api/keywords.yaml`).

## Flow

- The route compiles URL search params into filters (`compileSavedKeywordsFilters`),
  then calls `saved/list` with the filters, tag ids, sort and page.
- Bulk bar: remove (max 2000 ids) and tag add/remove via `saved/remove` /
  `saved/tags/assign`; tag manager uses `saved/tags/update|delete`. A 409
  `tag_in_use` on delete keeps the tag and names the count.
- Export: `saved/export` with the same filters, through the shared
  `exportRows` CSV/Sheets pipeline.
- Row tags render with the shared palette (`TagColorKey`); null color derives
  a stable color from the tag id.

## Rules that must never break

- Response shapes are Zod-validated; a drift surfaces as an error, not a
  broken table.
- Never break pagination/`keepPreviousData` behavior: the previous page stays
  on screen while the next loads.
- Selection resets on page/filter/sort change.
