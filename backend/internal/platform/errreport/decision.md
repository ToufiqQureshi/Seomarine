# Error reporting transport: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Sends sanitized application exceptions to Sentry when configured.

## Decisions to preserve

- Never report passwords, tokens, request bodies containing user content or provider credentials. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- Reporting failure must not fail the original user request or recursively report itself. _Why:_ Explicit failure behavior helps callers recover safely instead of treating an error as a valid result.
- Keep environment/config opt-in and bounded. _Why:_ Explicit bounds prevent a large or abusive request from consuming shared memory, latency or service capacity.

## Dependencies

Sentry endpoint/configuration; shared HTTP client.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
