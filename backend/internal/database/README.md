# Database schema

Goose migrations live under `migrations/`. Version 1 is the full legacy schema
(generated from the former Drizzle PostgreSQL migrations) plus the first Go
table, so a new database is built by `Migrate` alone. Versions 2 onward are Go
tables, all prefixed `go_`.

Do not edit version 1 once it has run anywhere; add a new numbered migration.

`testdata/legacy_schema.sql` is a trimmed copy of the legacy tables for Go
tests that run in a private schema. `database_test.go` checks that a fresh
database migrates end to end; keep every column in `legacy_schema.sql` present
in the migrations too.
