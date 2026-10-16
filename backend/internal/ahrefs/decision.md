# Ahrefs domain ratings: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Looks up free domain-rating values for bounded batches of normalized hosts.

## Decisions to preserve

- Keep batch size and host validation before outbound calls. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Do not use or describe this endpoint as paid backlink detail; detailed backlink reports use DataForSEO in `backlinks`. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Never log provider credentials or return raw transport errors. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.

## Dependencies

Ahrefs public domain-rating endpoint; no product database writes.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
