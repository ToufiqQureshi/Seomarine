# Razorpay adapter: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Wraps subscription creation/cancellation and webhook signature validation for billing.

## Decisions to preserve

- No raw payment secret in URL, logs or error responses. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- Use deadlines and validate signatures with constant-time comparison. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- Never let the adapter decide plan price or entitlement; billing catalog remains authoritative. _Why:_ A single server-owned source keeps UI access, checkout and limits aligned even if client state is stale or modified.

## Dependencies

Razorpay HTTP API and secret configuration.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
