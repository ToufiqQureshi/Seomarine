# Plan catalog: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Defines official Uplumo prices, entitlements and limits for pricing, billing and every feature gate.

## Decisions to preserve

- Never duplicate price/limits in frontend or feature code. _Why:_ A single server-owned source keeps UI access, checkout and limits aligned even if client state is stale or modified.
- Every advertised feature must be enforced, and Razorpay amount/plan ID must match the catalog. _Why:_ A single server-owned source keeps UI access, checkout and limits aligned even if client state is stale or modified.
- Plan limit changes need cost/unit-economics review and test coverage. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.

## Dependencies

Go catalog and Razorpay plan configuration.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
