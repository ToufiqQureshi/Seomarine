# Database and migrations: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Owns Postgres pool setup, goose migration application and persistent schema evolution.

## Decisions to preserve

- Migrations are forward-only and ordered uniquely; duplicate versions can prevent server startup. _Why:_ Migration and test mistakes can prevent startup or damage customer data, so ordering and isolation must remain explicit.
- New feature tables use `go_` prefixes and deliberate indexes/FKs/cascades. _Why:_ Consistent names prevent table collisions, and indexes/constraints keep ownership and lookup rules enforceable.
- Never use production data in tests; integration tests require explicit TEST\*DATABASE*URL. \_Why:* Migration and test mistakes can prevent startup or damage customer data, so ordering and isolation must remain explicit.

## Dependencies

Postgres/pgx and goose migrations.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
