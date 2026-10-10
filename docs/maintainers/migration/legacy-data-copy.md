# Legacy data copy

`copy-legacy-data.ts` copies rows from the configured D1 database into the
PostgreSQL schema created by the Goose baseline. It uses Wrangler's remote D1
JSON query mode and the `TARGET_DATABASE_URL` environment variable. The script
is dry-run by default; pass `--apply` only during an approved migration window.
It never deploys or modifies the D1 source. Copy runs require legacy writes to
be paused so offset paging sees a stable source. Inserts use `ON CONFLICT DO
NOTHING`, so a stopped run can be repeated without duplicating rows. Apply in
foreign-key order; cycles fail before any destination writes. Start with a dry
run and compare the per-table counts with the legacy database before applying.

Run `pnpm legacy:copy` to inspect counts, then `pnpm legacy:copy --apply` during the migration window. The migration PR does not run this tool.
