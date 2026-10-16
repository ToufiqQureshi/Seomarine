# Project lifecycle: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Lists, creates, reads, updates, archives, restores and resolves a project’s organization and market.

## Decisions to preserve

- Treat every project ID as untrusted and scope by the caller’s organization. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.
- Create/restore must enforce current plan limits under concurrency where required. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- Archiving must not erase customer data; restore must reconnect feature access without changing ownership. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.

## Dependencies

Postgres projects and organization membership; plan catalog; projectcontext/dashboard consumers.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
