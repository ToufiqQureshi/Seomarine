# Saved keywords: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. The route restores filters/sort/page from URL search params and queries saved rows.
2. User selects rows to tag/remove or exports the current filtered set.
3. Go `keywords` applies the same project-scoped filters/pagination and returns nullable metrics.
4. The UI keeps prior page rows while a new page loads and resets selection when filters change.

## Implementation map

- `SavedKeywordsFilters.tsx`
- `useSavedKeywordsExport.ts`
- `savedKeywordsUtils.ts`
- `backend/internal/keywords/saved.go`

## API and service dependencies

Go package: `backend/internal/keywords`; data is stored in saved-keyword and tag tables.

## Cost, security and failure boundaries

- All reads/writes remain project-scoped; a foreign row ID should behave as not found.
- Bulk remove is bounded; tag changes must remain atomic and handle tags in use.
- Export uses the same filters as the visible table so it cannot leak unrelated rows.
