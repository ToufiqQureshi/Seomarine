# Shared provider and runtime clients: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Contains common HTTP/provider utilities and shared queue primitives used across product features.

## Decisions to preserve

- Provider transport must not bypass feature validation or cost authorization. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Always set timeout/body limits and close response bodies; do not log secrets. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- Keep metering and queue idempotency consistent across callers. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.

## Dependencies

HTTP; DataForSEO/OpenRouter/market/spend subpackages; Redis/Postgres queue adapters.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
