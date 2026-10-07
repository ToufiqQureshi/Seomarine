# Agent guidance

Every coding agent (Codex, Claude, Cursor, others) follows this file.

**Read `CLAUDE.md` first.** It is the product brief: what Seomarine is, who
it beats and why, the architecture decisions and the full code-quality
rules. Where the two files disagree, `CLAUDE.md` wins. Feature status and
priority live in `ROADMAP.md`.

## Non-negotiables

- **Go is the core.** Every new backend feature is Go, in `backend/`
  (handler → service → repository, standard library first, plain SQL
  through pgx, goose migrations, Go-owned tables prefixed `go_`).
- **No new Node/TypeScript backend work.** The TanStack/Cloudflare
  Workers app in `src/server/` is legacy: fix bugs there, port features
  out of it, never add backend features to it. The Go server proxies
  routes it doesn't own to the legacy app.
- **Frontend** is React (Vite) in `src/client/` and `src/routes/`. New
  screens call the Go API under `/api/v1/`.
- **Brand:** the product is Seomarine. Never add OpenSEO names, links,
  logos or copy. The only exception is the license attribution in
  `LICENSE-OPENSEO-MIT`, which must never be deleted.
- **Never commit secrets.** Production secrets live in Railway variables.
  Test fixtures must not look like real credentials.
- **One feature per PR**, referencing its `ROADMAP.md` number, with
  validation notes in the description. Update `ROADMAP.md` status in the
  same PR.

## Mandatory checks before every push

Go (`cd backend`), all must pass. CI runs the same list against a real
Postgres and Redis:

```sh
go mod tidy && git diff --exit-code -- go.mod go.sum
test -z "$(gofmt -l .)"
go vet ./...
staticcheck ./...
golangci-lint run ./...
TEST_DATABASE_URL=... TEST_REDIS_URL=... go test -race -count=1 -cover ./...
govulncheck ./...
test -z "$(deadcode ./...)"
```

Frontend (repo root): `pnpm ci:check` (prettier, knip, tsc, oxlint) and
`pnpm test`.

A failing check or test is a real bug until proven otherwise. Never skip,
weaken or delete a test to get green.

## Engineering principles

- Prefer simple, readable, flat code with minimal indirection.
- Search for existing implementations and installed libraries before
  writing a helper. In Go, hand-rolling what the standard library already
  does is a defect.
- Abstract only when it prevents real drift. No speculative layers.
- Keep product data normalized with explicit relationships. Don't encode
  relational data in JSON or text to avoid joins.
- Validate all untrusted input at the boundary: the Go handler, or Zod in
  TypeScript.
- In the legacy TypeScript app, keep schema changes compatible with both
  SQLite and Postgres, and use the existing TanStack Query, Router and Form
  patterns.
- shadcn components in `src/client/components/ui/` are built on Base UI,
  not Radix. Compose them with the `render` prop, not `asChild`.

## Testing

- A test exists to catch a real bug: core behavior or an edge case that
  can actually happen. Every test must be able to fail.
- Test behavior at the public entry point. Go: table-driven tests,
  `httptest` for handlers, a real Postgres and Redis for repositories.
  Never mock SQL.
- When fixing a bug, first add the test that reproduces it.
- TypeScript (Vitest): import the module under test statically, set
  default mock values in `beforeEach` only, keep fixtures to the fields the
  test asserts on, and don't export a function only so a test can reach
  it (knip fails CI on it).
- A negative test must fail for the reason its name gives, not because an
  earlier guard rejected first.

## Documentation

- `docs/` (except `docs/maintainers/`) is user-facing: only what helps
  users understand, use or self-host Seomarine.
- `docs/maintainers/` holds engineering decisions and maintenance notes.
  Never put secrets there.
- Log small, recurring repository friction in `.agents/PAPERCUTS.md` with
  the `papercuts` skill. Real bugs are not papercuts.

## Review control plane

Changes to `AGENTS.md`, `CLAUDE.md`, `.claude/`, `.agents/skills/**` and
`.github/**` change how agents and CI work. Call them out in the PR so the
owner reviews them explicitly.
