# Shared Postgres pool and transactions: flow

> Internal guide for developers and coding agents.

## Current behavior

Startup calls `Open` and validates connectivity; repository code passes a transaction callback to `InTx`; callback errors roll back with a short uncanceled cleanup deadline.

## Dependencies

Postgres/pgx.

## Relevant code

- `pool.go`
- `pool_test.go`
