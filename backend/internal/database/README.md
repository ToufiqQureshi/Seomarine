# Database schema

Goose migrations live under `migrations/`. The Drizzle PostgreSQL schema
snapshot is staged under `baseline/00001_drizzle_baseline.sql`. It has Goose
markers and includes the original version 1 Go analytics table, but it is not
embedded or applied by `Migrate` yet. Test databases and existing deployments
already create legacy tables before Go migrations; replacing the active version
1 migration now would fail on duplicate tables. Do not move the staged baseline
into `migrations/` until the schema cutover and its test setup are ready.

The migration snapshot is sourced from Drizzle PostgreSQL migrations because
legacy tables are shared by the TypeScript application and Go services during
the transition. `drizzle/pg` remains the comparison source until the TypeScript
writer is retired and the schema copy tool has completed. At cutover, validate
the staged baseline against a fresh Postgres database and the existing schema,
then replace version 1 as part of a coordinated migration PR.
