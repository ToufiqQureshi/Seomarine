# SAM assistant: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Streams project-aware AI answers and tool actions with persisted sessions and credit controls.

## Decisions to preserve

- Model/tool usage can incur variable cost; enforce per-turn/daily limits, reserve/settle credits and stop at the ceiling. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Tool arguments cannot choose another project or bypass project membership. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.
- Provider errors must preserve already-produced partial output while charging actual usage only. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.
- Avoid refetching or replaying a streamed POST automatically. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.

## Verification pointers

- `samApi.test.ts`
