# Shared Postgres pool and transactions: decisions

> Internal guide for developers and coding agents. Update with code changes.

## Responsibility

Opens and verifies a pgx pool and provides transaction commit/rollback helper.

## Decisions to preserve

- Do not hide connection/startup failures or use a transaction after it ends. _Why:_ Migration and test mistakes can prevent startup or damage customer data, so ordering and isolation must remain explicit.
- Always attempt bounded rollback even when the request context has been canceled. _Why:_ Explicit bounds prevent a large or abusive request from consuming shared memory, latency or service capacity.
- Keep pool sizing/timeouts in startup config, not per request. _Why:_ Migration and test mistakes can prevent startup or damage customer data, so ordering and isolation must remain explicit.

## Cost and risk boundary

Postgres/pgx.
