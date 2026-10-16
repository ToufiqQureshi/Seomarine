# Backlink research: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. User selects a project target and scope, then requests backlink data in the page search card.
2. `backlinksApi.ts` submits a scoped request; filters and pagination are represented in the query controls.
3. Go `backlinks` validates the normalized target and filters, checks plan access, calls DataForSEO and maps provider rows to UI contracts.
4. The UI renders the summary, history where supported, referring domains and top pages with explicit scope notes.

## Implementation map

- `BacklinksPage.tsx`
- `backlinksApi.ts`
- `ahrefsApi.ts`
- `backend/internal/backlinks/handler.go`
- `backend/internal/backlinks/service.go`

## API and service dependencies

Go packages: `backend/internal/backlinks`; optional free ratings use `backend/internal/ahrefs`; transport and usage metering use `backend/internal/platform/dataforseo`.

## Cost, security and failure boundaries

- Backlink lookups are paid: validation and plan checks must happen before provider spend.
- A subfolder or exact-page scope may not support every aggregate; preserve backend scope notes and null values.
- Never silently retry paid POST requests or present missing provider data as zero.
