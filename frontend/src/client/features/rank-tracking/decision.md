# Rank tracking: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Configures keywords, devices/locations and manual or scheduled rank checks with history.

## Decisions to preserve

- Rank checks consume provider credits; require a visible estimate/ceiling and enforce it again on the server. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.
- Scheduled checks must stop or pause on limits; resuming must not duplicate tasks or charges. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- A partial provider error remains a partial run, not fabricated position data. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.
- Opening tracker screens must not start a check. _Why:_ Passive viewing and editing should not create paid work or external side effects without the user asking for them.

## Verification pointers

- `RankTrackingFilters.test.ts`
