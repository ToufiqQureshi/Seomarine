# MCP API keys: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Creates, lists and revokes long-lived credentials used by external MCP clients.

## Decisions to preserve

- Never persist or log plaintext keys. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- Show the full key only in the create response and keep the 100-key/name bounds. _Why:_ Explicit bounds prevent a large or abusive request from consuming shared memory, latency or service capacity.
- A foreign key ID should look absent, not reveal another user’s credential. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.

## Dependencies

Better Auth-compatible key store; `backend/internal/mcp` verifier.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
