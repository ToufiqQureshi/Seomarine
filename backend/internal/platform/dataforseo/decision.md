# DataForSEO transport: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Sends validated paid task requests, parses provider responses and records usage by organization.

## Decisions to preserve

- Provider may bill rejected/failed tasks: validate request before transport and never add automatic retries to task-post calls. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Attribute every billable task to the organization causing it; usage-write failure behavior must remain explicit. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Never put credentials in URLs/logs; cap body/response sizes and enforce timeouts. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.

## Dependencies

DataForSEO API, platform spend guard, Postgres usage ledger, Redis worker/cache as caller requires.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
