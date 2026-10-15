# Database and migrations: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** Server startup migration path; no product HTTP route.
- **Source:** `database.go`, `migrations/*.sql`

## Request or job flow

Server loads database config, opens a bounded pgx pool and applies ordered goose migrations before serving requests; feature repositories use plain SQL.

## Dependencies and cost boundary

Postgres/pgx and goose migrations.

## Failure, security and data invariants

- Migrations are forward-only and ordered uniquely; duplicate versions can prevent server startup.
- New feature tables use `go_` prefixes and deliberate indexes/FKs/cascades.
- Never use production data in tests; integration tests require explicit TEST_DATABASE_URL.
