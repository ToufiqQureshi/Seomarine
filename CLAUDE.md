# Uplumo: agent guide

Every coding agent (Claude, Codex, Cursor, others) follows this file.
`AGENTS.md` only points here.

## 1. What to read, and when

| You are about to...                      | Read first                                                    |
| ---------------------------------------- | ------------------------------------------------------------- |
| Do anything                              | This file                                                     |
| Choose, scope or prioritise a feature    | `docs/PRODUCT.md`, `ROADMAP.md`                               |
| Touch a feature's code                   | That feature's `README.md` (next to its code), then its tests |
| Add or change a table                    | `backend/internal/database/README.md`                         |
| Add or change an HTTP endpoint           | `backend/api/<feature>.yaml` if it exists, the feature README |
| Run it locally                           | `docs/LOCAL_DEVELOPMENT.md`                                   |
| Deploy, or change a production setting   | `docs/DEPLOY.md`                                              |
| Change email (Loops)                     | `backend/internal/email/README.md`                            |
| Hit repo friction (flaky script, gotcha) | Log it in `.agents/PAPERCUTS.md` with the `papercuts` skill   |

If a doc and the code disagree, the code is the truth: fix the doc in the same
change.

## 2. Layout

```
backend/    Go server. cmd/server (the site), cmd/erase-user (privacy erasure),
            internal/<feature>/, internal/platform/ (shared pieces), api/ (OpenAPI).
frontend/   React + Vite SPA (pnpm). Built to frontend/dist and served by the Go server.
docs/       PRODUCT.md, DEPLOY.md, LOCAL_DEVELOPMENT.md, audits/.
Dockerfile, railway.json    one image: the Go server plus frontend/dist.
```

Migrations are goose SQL in `backend/internal/database/migrations`, applied by
the server at startup. Number a new one after the highest that exists; two
files with the same number stop the server from starting.

Every feature folder (Go package or frontend feature) has a `README.md`: what
it does, the flow, tables, env vars, the rules that must never break, edge
cases and production limits. Say **why**, not what the code already says.
Code and README change in the same PR.

## 3. How to write code

**Go is the backend.** There is no TypeScript backend: the React app is a
static SPA that the Go server serves and that calls `/api/v1/`. No Next.js.

- **Inbuilt first.** Use the standard library (`net/http`, `log/slog`,
  `context`, `encoding/json`, `errors`, `slices`, `maps`, `strings`, `sync`,
  `time`, `testing`, `net/http/httptest`) and what `go.mod` already has.
  Hand-rolling something the standard library does is a defect. Check
  `go doc` before writing a helper. A new dependency needs a reason in the PR.
- **Layering:** handler -> service -> repository. Handlers validate every
  input and map errors to HTTP. Services hold the rules. Repositories hold
  plain SQL (pgx, no ORM). New tables are prefixed `go_`.
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
  and short-lived caches. Secrets live in Railway variables, never in the repo
  or in chat.
- **External data:** DataForSEO stays behind `backend/internal/platform/dataforseo`.
  Never send a request the provider would reject, because failed tasks are billed.
- **Money:** the price a customer reads must be the same everywhere.
  `ProPriceINR` in `backend/internal/site/site.go`, `PRO_PRICE_INR` in the
  frontend and the amount on the Razorpay plan change together.
- **Brand:** the product is Uplumo. Never add the upstream name, links,
  logos or copy. The only mention is the license attribution
  (`LICENSE-OPENSEO-MIT`, never delete it).
- **Copy:** plain language, Hinglish where the market is India, no raw jargon
  without a one-line explanation. Never promise on a public page what the
  product does not do.

**Frontend** (`frontend/src/client` and `frontend/src/routes`): React with
Vite, TanStack Query/Router/Form patterns, shadcn on Base UI (compose with the
`render` prop, not `asChild`). Screens call the Go API under `/api/v1/`
through `apiRequest` and validate responses with Zod.

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

## 5. Checks before every push

Go, from `backend/` (all must pass; CI runs the same against a real Postgres
and Redis):

```sh
go mod tidy && git diff --exit-code -- go.mod go.sum
test -z "$(gofmt -l .)"
go vet ./...
staticcheck ./...
golangci-lint run ./...
TEST_DATABASE_URL=... TEST_REDIS_URL=... go test -p 1 -race -count=1 -cover ./...
govulncheck ./...
test -z "$(deadcode -test ./...)"
```

Frontend, from `frontend/`: `pnpm ci:check` (prettier, knip, tsc, oxlint) and
`pnpm test`. CI runs both.

Do not commit with a failing check, and do not commit secrets.

## 6. Working with the owner

The owner (Toufiq) decides product scope, pricing, branding and architecture.
Propose with a recommendation; do not decide silently.

- Keep answers short, in Hinglish, senior-dev tone. No long essays.
- Before planning a new feature, search the web for current competitor
  features, pricing and user complaints, and cite the sources in the PR.
- Ship each feature as its own PR. The description says what changed, the
  decisions the owner must confirm, how it was validated and what was _not_ run.
- Several agents work in this repository at once. Work on your own branch,
  and never commit another agent's uncommitted files.
- Changes to `CLAUDE.md`, `AGENTS.md`, `.claude/`, `.agents/skills/**` and
  `.github/**` change how agents and CI work. Call them out in the PR.
- Merging to `main` does not deploy. See `docs/DEPLOY.md`.

## 7. Documentation rules

- A feature's notes live in its own `README.md`, next to the code.
- `docs/` holds only what covers the whole product: the product brief, how to
  run it and how to deploy it. Dated reviews go in `docs/audits/`.
- Do not keep plans, migration history or status logs as documents. Status
  lives in `ROADMAP.md`, history lives in git.
- Never put secrets in a document.

## 8. Legal

`LICENSE` is proprietary (all rights reserved) for Uplumo's own work.
Upstream-derived code stays under MIT. Never delete `LICENSE-OPENSEO-MIT` or
the copyright notice it carries.

The public legal pages (`backend/internal/site/templates`) name the operator,
address and support contact. Change them only on the owner's word.
