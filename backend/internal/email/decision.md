# Transactional email transport: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Sends transactional email through Loops for verification, invitations and account workflows.

## Decisions to preserve

- No marketing enrollment or contact-list side effects from transactional sends. _Why:_ Collecting or retaining less identifying data reduces privacy exposure if logs, storage or exports are accessed improperly.
- Use request deadlines and context cancellation; never log recipient secrets or provider auth. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- Provider failure must be handled by the calling flow without claiming that mail was delivered. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.

## Dependencies

Loops transactional API and secret environment configuration.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
