# Dashboard response contracts: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Defines shared dashboard response types; request behavior is implemented by dashboardoverview and activation packages.

## Decisions to preserve

- Keep this package free of side effects and provider dependencies. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Change JSON fields only with corresponding client and contract-test updates. _Why:_ UI and external clients depend on this contract; changing it without coordinated callers creates avoidable breakage.
- Do not implement refresh behavior in a response-types package. _Why:_ Keeping this boundary prevents unrelated code or test helpers from changing production behavior or creating hidden work.

## Dependencies

Go package contracts only; no IO, provider calls or persistence.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
