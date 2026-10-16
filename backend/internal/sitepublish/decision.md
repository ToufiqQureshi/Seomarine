# Customer-site publishing: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Connects WordPress and publishes content-editor drafts after explicit authorization and user action.

## Decisions to preserve

- Publish is an external side effect; preview and draft save must never publish. _Why:_ Passive viewing and editing should not create paid work or external side effects without the user asking for them.
- Do not retry uncertain create-post calls blindly; use persisted result/idempotency safeguards. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- Keep credentials encrypted/redacted and sanitize content/schema before sending. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.

## Dependencies

WordPress REST API; Postgres site connection/publication/draft rows; plan catalog.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
