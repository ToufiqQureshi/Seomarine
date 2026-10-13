# Database schema

Goose migrations live under `migrations/`. Version 1 holds the tables the
product started with (users, sessions, organizations, projects, audits,
reports and the rest) plus the first analytics table, so a new database is
built by `Migrate` alone. Versions 2 onward add tables prefixed `go_`.

Never edit a migration that has run anywhere, and production has run all of
them: add a new file numbered after the highest one. Two files with the same
number stop the server from starting, so check `main` before picking a number
when several branches are open.

`testdata/legacy_schema.sql` is a trimmed copy of the legacy tables for Go
tests that run in a private schema. `database_test.go` checks that a fresh
database migrates end to end; keep every column in `legacy_schema.sql` present
in the migrations too.
