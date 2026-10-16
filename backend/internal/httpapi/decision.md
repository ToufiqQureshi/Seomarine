# HTTP routing and middleware: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Mounts public site, auth, API routes, shared session/project guards, tracker collection and SPA fallback.

## Decisions to preserve

- Public routes must be intentionally public; every private API path uses the correct session/project/role guard. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.
- Do not bypass middleware when adding a route or mount the same route twice. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.
- Keep errors free of secrets/driver details and preserve request deadlines/cancellation. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.

## Dependencies

Every feature handler, auth middleware, logging and dependency wiring.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
