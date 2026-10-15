# Search feature navigation: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Keeps navigation state and tab selection consistent across search-related pages.

## Decisions to preserve

- Do not put provider calls or authorization decisions in tab state. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Unknown/stale query params should resolve to a safe default instead of opening an invalid feature. _Why:_ UI and external clients depend on this contract; changing it without coordinated callers creates avoidable breakage.
- Keep URL state shareable and avoid resetting feature filters unnecessarily. _Why:_ UI and external clients depend on this contract; changing it without coordinated callers creates avoidable breakage.

## Verification pointers

- `useSearchTabs.test.ts`
