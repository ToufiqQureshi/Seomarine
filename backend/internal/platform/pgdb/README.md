# Shared Postgres pool and transactions

Opens and verifies a pgx pool and provides transaction commit/rollback helper.

Internal package guide: [flow.md](flow.md) describes the code path; [decision.md](decision.md) records invariants to preserve.

## Source map

- `pool.go`
- `pool_test.go`
