# Saved keywords: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Organizes researched keywords with project tags, filters, bulk actions and export.

## Decisions to preserve

- All reads/writes remain project-scoped; a foreign row ID should behave as not found. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.
- Bulk remove is bounded; tag changes must remain atomic and handle tags in use. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- Export uses the same filters as the visible table so it cannot leak unrelated rows. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.

## Verification pointers

- `savedKeywordsFilterTypes.test.ts`
- `savedKeywordsUtils.test.ts`
