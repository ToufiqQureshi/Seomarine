# gdpr

Erases one account and the data only that account owns, in one Postgres
transaction. Run it as `cmd/erase-user` (dry run by default).

```sh
DATABASE_URL=... go run ./cmd/erase-user -email person@example.com
DATABASE_URL=... go run ./cmd/erase-user -email person@example.com -execute \
  -confirm person@example.com -confirm-database-host <host printed by the dry run>
```

## How it works

1. Find the user, list the workspaces they belong to, and **refuse** if any has
   another member: erasing it would destroy that person's data. A human settles
   those first (transfer or remove the user).
2. Inventory: count the rows that reference the user, their workspaces and their
   projects, in every table that has a `user_id`, `organization_id` or
   `project_id` column. The list comes from `information_schema`, so a table
   added later is covered without touching this package.
3. Execute: delete reset tokens, the workspaces, the user (foreign keys cascade),
   then sweep every reference column for what the cascade did not reach, and
   check the user row is gone. Any error rolls everything back.

## Rules that must never break

- Dry run is the default. Executing needs the exact email **and** the database
  host printed by the dry run, so a wrong `DATABASE_URL` cannot erase the wrong
  database.
- A workspace with other members is never erased. A member who joins between the
  dry run and the erasure also aborts it.
- Nothing outside the one transaction is changed: there is no second store.
  (The TypeScript version also called a Worker to clear R2 and Durable Objects.
  The Go server has neither: audit payloads and SAM transcripts are Postgres rows
  that cascade.)

## Decision for the owner

The sweep erases billing ledger rows (`go_billing_*`) together with the
workspace. If tax or accounting rules require keeping payment records, exclude
those tables before this runs in production.
