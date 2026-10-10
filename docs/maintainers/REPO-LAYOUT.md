# Repo layout: target and the order to get there

Owner decision: the finished repo has three code folders, `frontend/`,
`backend/` and `db/`, plus `docs/`. Everything else moves into them or is
deleted. This file is the plan; `CLAUDE.md` says never to move folders
outside it.

## Target

```
CLAUDE.md  AGENTS.md  ROADMAP.md  README.md  LICENSE  LICENSE-OPENSEO-MIT
go.mod  go.sum                  Go module root (moved up from backend/)
Dockerfile  railway.json        one image: Go server + frontend/dist
.env.example  Makefile  compose.yaml  .github/

frontend/                       React + Vite SPA, its own package.json
  src/features/<feature>/       README.md per feature
  src/components/  src/lib/  src/routes/
  tests next to the code (Vitest)

backend/
  cmd/server/                   main.go: wiring only
  internal/<feature>/           handler, service, repository, tests, README.md
  internal/platform/            dataforseo, jobs, httpx, pgdb, kv, entitlements
  api/<feature>.yaml            OpenAPI: the contract with the frontend

db/
  db.go                         package db: //go:embed migrations/*.sql
  migrations/                   goose SQL, one table family per feature
  schema/                       <feature>.md table docs, legacy_schema.sql (test fixture)
  seed/                         local development data

docs/                           user-facing docs, docs/maintainers/ engineering notes
```

Why `go.mod` moves to the root: `go:embed` cannot reach outside its package
tree, so `db/` can only embed its migrations if it is inside the module.
Import paths do not change (`.../seomarine/backend/internal/x` stays the same
string once the module is `github.com/toufiqqureshi/seomarine`).

`legacy/` is a temporary fourth folder for today's TypeScript app. It exists
only while features are being ported, shrinks with every PR, and is deleted
with the last port. The finished repo has no `legacy/`.

## Where things are today

| Today                                                                                          | Target                         | When                                           |
| ---------------------------------------------------------------------------------------------- | ------------------------------ | ---------------------------------------------- |
| `backend/go.mod`, `backend/go.sum`                                                             | repo root                      | Step 1                                         |
| `backend/internal/database/migrations`                                                         | `db/migrations` (+ `db/db.go`) | Step 1                                         |
| `backend/internal/database/testdata/*.sql`                                                     | `db/schema/legacy_schema.sql`  | Step 1                                         |
| `src/client`, `src/routes`, parts of `src/shared`                                              | `frontend/src`                 | Step 4: after the last server function is gone |
| `src/server`, `src/serverFunctions`, `drizzle`, `wrangler*.jsonc`, `deploy/alchemy`, `scripts` | `legacy/`, then deleted        | Step 3, shrinking until the end                |
| `backend/internal/<feature>`                                                                   | stays                          | n/a                                            |

## Order

1. **Go module root + `db/`.** Move `go.mod` up, add `db/db.go`, move
   migrations and the test fixture, add `db/schema/<feature>.md`. Needs PRs
   #20 and #21 merged first, because they edit `go.mod`.
2. **Root files.** `Dockerfile` (multi-stage: build frontend, build Go, one
   binary serving `frontend/dist`), `railway.json`, `.env.example`, `Makefile`
   (`make check` runs every check for every folder), CI path filters per folder.
3. **Put the TypeScript app under `legacy/`.** Mechanical move plus path fixes
   in CI and configs, no behavior change.
4. **Extract `frontend/`.** The legacy app is one TanStack Start build, so
   the React client cannot leave it while any page still calls a TypeScript
   server function. Convert to a plain Vite SPA only when the last
   `createServerFn` is replaced by a Go endpoint (ROADMAP 0.3 to 0.4).
5. **Port features** in dependency order, one PR each: `backlinks` -> `ga4` ->
   `gsc` -> `google` -> `reports` -> `sam` -> `dashboard` + `activation` +
   `referrals` -> `audit` -> `billing` -> `gdpr` last. Bundle features that
   share TypeScript consumers into one PR.
6. **Delete `legacy/`.**

## Not on the old list, but required before `legacy/` can go

Features and platform pieces that are easy to forget:

- **Login and sessions.** Go only reads better-auth's session today. Sign up,
  sign in, OAuth, password reset and organization/member management must move.
- **Email.** Transactional and lifecycle mail (Loops).
- **Background work.** Cloudflare Workflows and cron (audits, rank checks,
  scheduled reports) become jobs on `internal/platform/jobs`.
- **Object storage.** R2 usage (caches, exports, uploads) needs a Railway bucket
  or S3 equivalent; the privacy erasure job must cover it and Redis.
- **MCP server** (`src/server/mcp`, about 9k lines) and the ChatGPT app.
- **Credits and usage.** Autumn credits become the Razorpay plan plus the
  `go_dataforseo_usage` ledger; "live usage" is a pricing promise (ROADMAP 5.3).
- **Observability.** Error reporting and a `/metrics` or log pipeline.
- **A schema baseline.** The legacy tables come from Drizzle. Once TypeScript
  stops writing, capture them in one goose baseline migration so `db/` is the
  only owner of the schema. The version 1 snapshot is prepared under
  `backend/internal/database/migrations/00001_drizzle_baseline.sql`; it includes
  the prior Goose v1 analytics table so deployed version history stays stable.
- **API contract checks.** Generate TypeScript types from `backend/api/*.yaml`
  and fail CI when the frontend and an endpoint disagree.
- **Self-hosting docs** for the single-binary deployment.

## Rules while moving

- One mechanical move per PR, no behavior change in it.
- Update `CLAUDE.md`'s read map and this table in the same PR as the move.
- CI must stay green on every PR; never leave a folder half moved.
