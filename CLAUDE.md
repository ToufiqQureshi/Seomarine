# Uplumo: agent guide

Every coding agent (Claude, Codex, Cursor, others) follows this file.
`AGENTS.md` only points here.

## 0. Where Uplumo is today

- Live at `https://uplumo.com` on Railway (one Go server that also serves
  the React build, plus Postgres and Redis). Merging to `main` does **not**
  deploy: see `docs/DEPLOY.md`.
- Razorpay is connected with **test-mode** keys: checkout works with test
  cards, no real money moves yet. Live keys go in only after a full test
  payment, renewal and cancel on the site.
- `DATAFORSEO_API_KEY` and `OPENROUTER_API_KEY` are not set in production,
  so paid data and AI answer `503` there.
- One person runs the company. You are the engineering team, QA, security,
  SRE and the person who thinks about money: act like it (section 3).

The current status of every feature is in `ROADMAP.md`; check it and the
code before you trust this section.

## 1. What to read, and when

| You are about to...                      | Read first                                                    |
| ---------------------------------------- | ------------------------------------------------------------- |
| Do anything                              | This file                                                     |
| Choose, scope or prioritise a feature    | `docs/PRODUCT.md`, `ROADMAP.md`                               |
| Check what the owner already decided     | `DECISIONS.md` (decisions, Railway variable names, open asks) |
| Touch a feature's code                   | That feature's `README.md` (next to its code), then its tests |
| Add or change a table                    | `backend/internal/database/README.md`                         |
| Add or change an HTTP endpoint           | `backend/api/<feature>.yaml` if it exists, the feature README |
| Run it locally                           | `docs/LOCAL_DEVELOPMENT.md`                                   |
| Deploy, or change a production setting   | `docs/DEPLOY.md`                                              |
| Change email (Loops)                     | `backend/internal/email/README.md`                            |
| Hit repo friction (flaky script, gotcha) | Log it in `.agents/PAPERCUTS.md` with the `papercuts` skill   |

If a doc and the code disagree, the code is the truth: fix the doc in the same
change.

### Docs map

Every Markdown file in the repo is listed here; read only the ones your
change touches. A test (`frontend/scripts/docs-map.test.ts`) fails when a new
`.md` file is missing from this list, so add the line in the same change.
Skills (`.agents/skills/`, mirrored in `.claude/skills/`) are listed by the
harness and are not repeated here.

Whole product: `README.md` (what Uplumo is), `ROADMAP.md` (status),
`docs/PRODUCT.md` (why, for whom), `docs/LOCAL_DEVELOPMENT.md`,
`docs/DEPLOY.md`, `docs/audits/` (dated reviews, read only when asked),
`backend/README.md` (Go server, Razorpay billing rules, checks),
`.agents/PAPERCUTS.md` (known repo friction), `AGENTS.md` and
`.claude/CLAUDE.md` (both point here).

Go features, `backend/internal/…/README.md`, read before touching that package:

| Package                                    | Covers                                              |
| ------------------------------------------ | --------------------------------------------------- |
| `backend/internal/activation/README.md`    | Dashboard first-run checklist state                 |
| `backend/internal/aisearch/README.md`      | AI Visibility: Brand Lookup, Prompt Explorer        |
| `backend/internal/analytics/README.md`     | Uplumo Analytics tracker, ingest, AI traffic        |
| `backend/internal/analytics/geo/ATTRIBUTION.md` | DB-IP country database licence (keep)          |
| `backend/internal/apikeys/README.md`       | API keys for MCP clients                            |
| `backend/internal/audit/README.md`         | Site audit, Lighthouse via PageSpeed, CrUX field data |
| `backend/internal/auth/README.md`          | Session reading, auth middleware                    |
| `backend/internal/backlinks/README.md`     | Backlinks pages                                     |
| `backend/internal/billing/README.md`       | Plans, credits ledger, spend guard, cancelling      |
| `backend/internal/dashboardoverview/README.md` | Dashboard audit and backlink cards              |
| `backend/internal/database/README.md`      | Migrations and table rules                          |
| `backend/internal/domain/README.md`        | Domain Overview                                     |
| `backend/internal/email/README.md`         | Loops email                                         |
| `backend/internal/emailauth/README.md`     | Sign-up, sign-in, verification, reset, Google login |
| `backend/internal/ga4/README.md`           | Google Analytics 4 reports                          |
| `backend/internal/gdpr/README.md`          | Account erasure                                     |
| `backend/internal/google/README.md`        | Google OAuth connections and tokens                 |
| `backend/internal/gsc/README.md`           | Search Console                                      |
| `backend/internal/keywords/README.md`      | Keyword research, saved keywords, locations         |
| `backend/internal/mcp/README.md`           | `/mcp` server and its tools                         |
| `backend/internal/plans/README.md`         | Plan catalog: prices, limits, features              |
| `backend/internal/platform/dataforseo/README.md` | DataForSEO transport                          |
| `backend/internal/platform/errreport/README.md`  | Sentry error reporting                        |
| `backend/internal/platform/market/README.md`     | Countries and languages                       |
| `backend/internal/platform/spend/README.md`      | Spend hold contract for paid providers        |
| `backend/internal/projectcontext/README.md` | Project memory shared by UI, MCP, SAM              |
| `backend/internal/projects/README.md`      | Project API                                         |
| `backend/internal/ranktracking/README.md`  | Rank tracking                                       |
| `backend/internal/reports/README.md`       | Reports and share links                             |
| `backend/internal/sam/README.md`           | SAM in-app agent                                    |
| `backend/internal/sam/product_fact_sheet.md` | Facts SAM may state about Uplumo (keep true)      |
| `backend/internal/sitepublish/README.md`   | WordPress connection and publishing                 |
| `backend/internal/site/blog/README.md`     | Public blog: how to add a post, SEO checklist       |

Frontend features, `frontend/src/client/features/…`:
`content-editor/README.md`, `keywords/README.md`, `saved-keywords/README.md`,
`site-publishing/README.md`, and `ai-mcp/agentSetupPrompt.md` (the prompt
users paste into their agent; it is product copy).

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

### Before you write a line

1. **Is it needed?** Build what a real customer or a real failure needs,
   nothing for an imagined future. No unused parameters, options, layers,
   files or "just in case" branches. Under-engineering is a defect too: a
   missing timeout, limit, check or test is not "simple".
2. **Does it already exist?** Look in this repo, then the standard library,
   then `go.mod` and `package.json`. Read the current docs before you code:
   Go release notes (`go.dev/doc/go1.27`, e.g. `errors.AsType`,
   `sync.WaitGroup.Go`, `testing.T.Context`, `slices`, `maps`,
   `strings.SplitSeq`, the `uuid` package) and `go doc`; for the frontend the
   React 19, TanStack Query/Router/Form, Zod 4, Base UI and Tailwind 4 docs.
   Search the web for the established way when you are not sure. A
   hand-rolled copy of something that exists is a defect.
3. **Is this the right way?** Prefer the boring, documented approach. If two
   ways work, pick the one with less code and fewer moving parts, and say
   why in the PR.
4. **What can it cost us?** For every change that touches money, data,
   providers or other customers, write down the scenarios and how the code
   stops each one from losing money or trust, then test them:
   - a bug or loop calls a paid provider again and again;
   - one customer's heavy use slows, blocks or bills another;
   - a provider, Postgres or Redis is down, slow or answers garbage;
   - the same request, webhook or job runs twice (retries, double clicks);
   - two requests race for the same row, credit or limit;
   - a user reaches another workspace's data;
   - a public page promises something the product does not do.
     Put the list, and what you did about each, in the PR.
5. **Is every line production-ready?** Errors wrapped and handled, inputs
   validated, timeouts and limits on every outside call, idempotent where it
   can repeat, logged where an on-call person needs it, and a way back if it
   goes wrong (migrations only move forward, so design for that).

Then delete every line that does not earn its place.

### Mistakes we already made (do not repeat them)

- **One shared limit for every customer.** A global $25/day stop meant one
  heavy workspace paused all the others, and they would leave. A limit that
  protects Uplumo must be scoped to whoever causes the cost (per workspace,
  per plan); a global stop is only an emergency brake, set far above normal
  use. Before you ship any limit, work it with real numbers: "5 customers on
  Pro, one uses 10x the others: who gets blocked, when, and what do they see?"
- **Building before recommending.** Writing code first and explaining later
  hides the decision from the owner. When there is an established tool or
  pattern (TanStack Query for server state instead of `useEffect` fetches,
  `errors.AsType`, a provider's own limit or webhook), say so **before**
  coding, with the trade-off, and let the owner choose.
- **Rules that look safe but punish paying users.** A check that fails
  closed is right; a check that blocks a paying customer for something they
  did not do is a bug. Ask "who does this stop, and is that the person who
  caused the problem?"

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
- **Plans:** prices, limits and what each plan includes live in one place,
  `backend/internal/plans/catalog.go`. The pricing page, the Billing screen
  and every plan check read it; never copy a price or a limit anywhere else.
  The amount on each Razorpay plan must equal the catalog's price.
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

- Speak up first. If you see a better tool, a risk, or a cheaper way, say it
  before you build, even when nobody asked. Silence about a known problem is
  a defect.
- Anything that touches money, limits, pricing or every customer at once:
  post the plan and the real-number scenario to the owner and wait for a yes
  before coding.

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
- The one exception is `DECISIONS.md`: the owner's decisions, which Railway
  variables are set (names only) and what waits on the owner. Update it in the
  same change when one of those changes.
- Never put secrets in a document.

**Docs change with the code, in the same commit.** A PR that changes
behaviour, a rule, an env var, a limit, an endpoint or a table also updates
the README of that feature (and `ROADMAP.md`, `docs/DEPLOY.md` or the Docs
map when they are affected). A stale doc is a bug: the next agent trusts it.
Before pushing, ask "which doc says something this diff just made untrue?"

**Write docs for an agent that has never seen the code:**

- Lead with what the thing does and when to read the doc, in one or two
  sentences.
- Then the rules that must never break, the flow, env vars and edge cases,
  as short bullets or tables, not essays.
- Say **why** a rule exists; the code already says what it does.
- Name exact files, functions, tables, routes and env vars, so they can be
  searched. Give real numbers (limits, timeouts, prices), not "a few".
- One fact in one place; link to it instead of copying it.
- No history, dates or "we changed X" in a README; git keeps that.

## 8. Legal

`LICENSE` is proprietary (all rights reserved) for Uplumo's own work.
Upstream-derived code stays under MIT. Never delete `LICENSE-OPENSEO-MIT` or
the copyright notice it carries.

The public legal pages (`backend/internal/site/templates`) name the operator,
address and support contact. Change them only on the owner's word.
