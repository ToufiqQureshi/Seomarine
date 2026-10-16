# Onboarding state: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Stores first-run answers and Search Console nudge state for the signed-in user.

## Decisions to preserve

- Scope answers to the current user and keep dismissal independent from actual integration status. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.
- Repeated save/dismiss requests are idempotent. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- Onboarding state must never grant project access or bypass plan checks. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.

## Dependencies

Postgres onboarding records; auth session; Google integration owns OAuth.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
