# AI Visibility and Prompt Explorer: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Runs Brand Lookup and Prompt Explorer, shapes answer-engine data, and caches successful results.

## Decisions to preserve

- Plan and request validation must happen before provider spend. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Keep all billed calls attributed to the project organization; never blindly retry paid POSTs. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- Cache only successful complete results; preserve per-platform partial failures and safe citation URLs. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.
- A cached prompt may contain user-entered text; retain expiry and privacy-erasure limitations in the feature README. _Why:_ Prompts can include customer research, so cache expiry and erasure behavior limit retention of that content.
- Save history only from a complete, data-bearing Brand Lookup and key it with organization, project and every lookup option that changes the result. _Why:_ Partial results must not be frozen as a full monthly trend or mixed across customer scopes.
- History reads use Redis aggregates and are best effort; they must not call the paid provider or fail the current lookup. _Why:_ Viewing an existing result should not create another charge or turn a secondary chart outage into a failed core lookup.
- Retain the 13-month limit and 400-day Redis expiry. _Why:_ A fixed retention window bounds storage and avoids retaining user research indefinitely.

## Dependencies

Redis cache; DataForSEO AI endpoints; plans/billing.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
