# Keyword research: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Researches seed terms and SERP results for a project market, with location selection and keyword metrics.

## Decisions to preserve

- Research and SERP calls can be billed by the provider; reject invalid requests before spend and do not add blind POST retries. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Cache keys must include every option that changes results, project and organization scope. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.
- Show absent/null provider metrics as unavailable, not as zero. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.
- Location results must use canonical provider locations; do not silently fall back to a different geography. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.

## Verification pointers

- `useKeywordResearchData.test.ts`
- `groupSharedVolumeRows.test.ts`
