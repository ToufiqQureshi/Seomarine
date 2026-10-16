# Google Analytics reports: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Reads project-selected GA4 reports, organic overview, measurement health and GA4/GSC search opportunities.

## Decisions to preserve

- Google calls are quota-limited; use bounded reads/retries and avoid duplicate queries. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- Never expose OAuth tokens; property must belong to the connected organization/project. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- GA4 and GSC metrics differ in definitions/attribution; do not silently equate them. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.

## Dependencies

Google Analytics Admin/Data APIs and GSC; encrypted Google tokens; Postgres selected property.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
