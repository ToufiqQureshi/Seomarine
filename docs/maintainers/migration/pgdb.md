# PostgreSQL pool migration

Status: IN PROGRESS

This Phase A item centralizes creation and health-checking of the shared pgx
pool and transaction commit/rollback handling in `internal/platform/pgdb`.

The server and database-backed test setup use `pgdb.Open`. Schema migrations
remain in `internal/database`; legacy table schemas are unchanged.

Tests cover a real PostgreSQL connection, invalid and unreachable URLs,
transaction commit, and callback-error rollback. The real-Postgres tests run in
Linux CI because this Windows host's Application Control blocks some Go test
executables even when `GOTMPDIR` is set to `D:\gotmp`.

PR: pending.
