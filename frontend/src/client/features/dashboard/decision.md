# Project dashboard: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Combines project onboarding state, audit/backlink summaries and GA4 overview cards.

## Decisions to preserve

- Dashboard loads must not trigger expensive provider refreshes implicitly; refresh stays an explicit action. _Why:_ Passive viewing and editing should not create paid work or external side effects without the user asking for them.
- Refresh cost and plan checks stay server-side; stale snapshots should be labeled, not silently refreshed. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Activation mutations are user/project-scoped and first-use timestamps remain stable. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.

## Verification pointers

- `dashboardApi.test.ts`
- `dashboardSteps.test.ts`
