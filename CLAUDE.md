# Seomarine: agent guide

Every coding agent (Claude, Codex, Cursor, others) follows this file.
`AGENTS.md` only points here. Product reasoning is in
`docs/maintainers/PRODUCT.md`; feature status is in `ROADMAP.md`.

## 1. What to read, and when

| You are about to...                      | Read first                                                                             |
| ---------------------------------------- | -------------------------------------------------------------------------------------- |
| Do anything                              | This file                                                                              |
| Choose, scope or prioritise a feature    | `docs/maintainers/PRODUCT.md`, `ROADMAP.md`                                            |
| Touch a feature's code                   | That feature's `README.md` (next to its code), then its tests                          |
| Port a feature from TypeScript to Go     | Section 5 below, the feature's TS code, then `docs/maintainers/REPO-LAYOUT.md`         |
| Move files or folders                    | `docs/maintainers/REPO-LAYOUT.md`                                                      |
| Add or change a table                    | `db/` (or `backend/internal/database/migrations` until the move), the feature's README |
| Add or change an HTTP endpoint           | `backend/api/<feature>.yaml`, the feature's README                                     |
| Review a PR                              | `docs/maintainers/review-guidelines.md`                                                |
| Deploy or run locally                    | `backend/README.md`, `docs/LOCAL_DEVELOPMENT.md`, `docs/maintainers/runbooks/`         |
| Hit repo friction (flaky script, gotcha) | Log it in `.agents/PAPERCUTS.md` with the `papercuts` skill                            |

If a README and the code disagree, the code is the truth: fix the README in
the same change.

## 2. Layout

Target: three code folders, plus docs.

```
frontend/   React + Vite SPA (pnpm/npm). Built to frontend/dist, served by the Go server.
backend/    Go server: cmd/, internal/<feature>/, internal/platform/, api/ (OpenAPI).
db/         migrations/ (goose SQL), schema/ (per-feature table docs, test fixtures), seed/.
docs/       user-facing docs, plus docs/maintainers/ for engineering notes.
legacy/     TEMPORARY. Today's TypeScript app. Shrinks per ported feature, then is deleted.
```

Today the repo is mid-move: the React app and the legacy server still live in
`src/`, and migrations in `backend/internal/database/migrations`. The plan and
the order of moves are in `docs/maintainers/REPO-LAYOUT.md`. Never move
folders outside that plan.

Every feature folder (Go package or frontend feature) has a `README.md`: what
it does, the flow, tables, env vars, the rules that must never break, edge
cases and production limits. Say **why**, not what the code already says.
Code and README change in the same PR.

## 3. How to write code

**Go is the core.** Every new backend feature is Go. No Next.js. No new
TypeScript backend work: the legacy server only gets bug fixes and is ported
out feature by feature. The Go server proxies every route it does not own yet
to the legacy app.

- **Inbuilt first.** Use the standard library (`net/http`, `log/slog`,
  `context`, `encoding/json`, `errors`, `slices`, `maps`, `strings`, `sync`,
  `time`, `testing`, `net/http/httptest`) and what `go.mod` already has.
  Hand-rolling something the standard library does is a defect. Check
  `go doc` before writing a helper. A new dependency needs a reason in the PR.
- **Layering:** handler -> service -> repository. Handlers validate every
  input and map errors to HTTP. Services hold the rules. Repositories hold
  plain SQL (pgx, no ORM). Go tables are prefixed `go_`.
- **Errors:** never ignore one. Wrap with context
  (`fmt.Errorf("load project %s: %w", id, err)`), check with `errors.Is` and
  `errors.AsType`. No panics for control flow.
- **Context and time:** pass `context.Context` through every request path and
  set a timeout on every outbound call.
- **No global mutable state.** Make concurrency explicit (`sync`, channels)
  and race-free.
- **Simple, flat, readable.** Delete any line that does not earn its place.
  Abstract only when it prevents real drift.
- **Data:** Postgres is the primary store, normalized. Redis holds rate limits
  and short-lived caches. ClickHouse only when analytics volume needs it.
  Secrets live in Railway variables, never in the repo.
- **External data:** DataForSEO stays behind `backend/internal/platform/dataforseo`.
  Never send a request the provider would reject, because failed tasks are billed.
- **Brand:** the product is Seomarine. Never add the upstream name, links,
  logos or copy. The only mention is the license attribution
  (`LICENSE-OPENSEO-MIT`, never delete it).
- **Copy:** plain language, Hinglish where the market is India, no raw jargon
  without a one-line explanation.

**Frontend** (`frontend/`, today `src/client` and `src/routes`): React with
Vite, TanStack Query/Router/Form patterns, shadcn on Base UI (compose with the
`render` prop, not `asChild`). New screens call the Go API under `/api/v1/`
through `apiRequest` and validate responses with Zod. In the legacy TypeScript
app, keep schema changes compatible with SQLite and Postgres.

## 4. How to write tests

A test exists to **catch a bug**, not to turn CI green.

1. **Test behavior at the public entry point.** Go: table-driven, `httptest`
   for handlers, a real Postgres and Redis for repositories (never mock SQL),
   an in-process fake (`httptest.Server`) for outside providers.
2. **Cover what can actually happen:** empty, nil, max size, unicode, timeouts,
   concurrency, and every error path (including the provider failing, the cache
   being down, the caller going away).
3. **Every test must be able to fail.** After writing tests, break the code
   under test on purpose (flip a condition, drop a check) and confirm a test
   fails. A mutation that survives means a missing test: add it.
4. **A failing test is a real bug until proven otherwise.** Find out whether
   the code or the test is wrong, and fix the cause. Never skip, weaken,
   delete or loosen a test to get green.
5. **Bug fix:** write the test that reproduces it first, watch it fail, then fix.
6. A negative test must fail for the reason its name gives, not because an
   earlier guard rejected first.
7. Test fixtures must not look like real credentials (secret scanners flag
   them, and rewriting history is the only cure).
8. Vitest (frontend): import the module under test statically, set default mock
   values in `beforeEach` only, keep fixtures to the fields asserted on, and do
   not export a function only so a test can reach it (knip fails on it).

## 5. Porting a feature from TypeScript to Go

One feature per PR, with its `ROADMAP.md` number.

1. Read the TS code, its tests and its callers. List behaviors and edge cases.
2. Write the Go package (handler, service, repository), keeping the JSON
   contract so the page changes only its call site.
3. Port the TS tests, then add the cases the TS never had. Run the mutation check.
4. Switch the page to the Go API. Remove the proxy route.
5. Delete the TS code that became dead (`knip` finds it). Code that other
   features still import stays, and the PR says so.
6. Write the feature `README.md`. Update `ROADMAP.md`.
7. PR description: what changed, decisions the owner must confirm, how it was
   validated, what was _not_ run.

Do not change behavior silently while porting. A deliberate difference goes in
the README and the PR.

## 6. Checks before every push

Go (all must pass; CI runs the same against a real Postgres and Redis):

```sh
go mod tidy && git diff --exit-code -- go.mod go.sum
test -z "$(gofmt -l .)"
go vet ./...
staticcheck ./...
golangci-lint run ./...
TEST_DATABASE_URL=... TEST_REDIS_URL=... go test -race -count=1 -cover ./...
govulncheck ./...
test -z "$(deadcode -test ./...)"
```

Frontend: `pnpm ci:check` (prettier, knip, tsc, oxlint) and `pnpm test`.

Run them from the Go module root (`backend/` until `REPO-LAYOUT.md` moves
`go.mod`). Do not commit with a failing check, and do not commit secrets.

## 7. Working with the owner

The owner (Toufiq) decides product scope, pricing, branding and architecture.
Propose with a recommendation; do not decide silently.

- Keep answers short, in Hinglish, senior-dev tone. No long essays.
- Before planning a new feature, search the web for current competitor
  features, pricing and user complaints, and cite the sources in the PR.
- Ship each feature as its own PR with validation notes.
- Changes to `CLAUDE.md`, `AGENTS.md`, `.claude/`, `.agents/skills/**` and
  `.github/**` change how agents and CI work. Call them out in the PR.

## 8. Documentation rules

- `docs/` (except `docs/maintainers/`) is user-facing: only what helps users
  understand, use or self-host Seomarine.
- `docs/maintainers/` holds engineering decisions and notes. Never put
  secrets there.

## 9. Legal

`LICENSE` is proprietary (all rights reserved) for Seomarine's own work.
Upstream-derived code stays under MIT. Never delete `LICENSE-OPENSEO-MIT` or
the copyright notice it carries.
