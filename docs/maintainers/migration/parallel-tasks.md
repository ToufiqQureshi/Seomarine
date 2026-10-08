# Seomarine: parallel task pack

How to use: open one AI session per block. Paste COMMON first, then ONE block
(PHASE A or one LANE). Run PHASE A alone first and get it merged. After that,
start all lanes at the same time, each in its own git worktree.

Honest expectation: PHASE A takes ~3-4 hours. After it merges, each lane's
first mini-task (M1) lands in ~3-5 hours. With 6+ lanes running, about
35-50% of all PRs can land in day 1 and ~70% by the end of day 2, if CI is not
the bottleneck. 70% in one single day is only possible if the lanes never
block on each other, so the rules below exist to make them independent.

---

## COMMON (paste at the top of every session)

You are an engineer on Seomarine. Goal of the whole program: ALL backend logic
moves from TypeScript (`src/server`, `src/serverFunctions`, `src/db`) to Go
(`backend/`). TypeScript stays only as the React frontend that calls
`/api/v1/*`. Several other AI sessions work at the same time in other lanes.
Your job is ONE lane. Stay inside it.

Read first (max 10 min): `CLAUDE.md`, `AGENTS.md`, `docs/maintainers/migration/README.md`
(if present), and the legacy TS code of your lane.

No web search. No deployment work. No new product features (except where your
lane says so). Faithful port: same behavior, same data, same UI.
Brand is Seomarine only. Never add "OpenSEO". Never touch `LICENSE-OPENSEO-MIT`.
Never commit secrets. Never skip, weaken or delete a test to get green.

### Mini-task rule

Every mini-task (M1, M2, ...) is ONE branch and ONE draft PR, under ~800 changed
lines (excluding generated code and deleted TS). Branch name:
`mig/<lane>-<mini-task>` from latest `main`. Finish it, open the PR, then start
the next mini-task of your lane immediately. Do not wait for review.

### Structure (feature-wise)

```
backend/internal/<feature>/   handler.go service.go repository.go types.go <feature>_test.go [jobs.go]
backend/internal/platform/    shared building blocks (OWNED BY PHASE A; do not edit)
backend/api/<feature>.yaml    OpenAPI of this feature (one file per feature)
src/client/features/<feature>/  api.ts, query hooks, components
docs/maintainers/migration/<feature>.md   progress of this feature (one file per feature)
```

Each feature exposes `func Mount(mux *http.ServeMux, d Deps)` and registers its
own routes. The root router only calls each `Mount` (one line per feature, in
alphabetical order).

### Anti-conflict rules (many sessions run in parallel)

1. Touch ONLY: your feature folders, your `backend/api/<feature>.yaml`, your
   `src/serverFunctions/<your files>`, your `src/client/features/<your>`, your
   `docs/maintainers/migration/<feature>.md`. If you must change anything else
   (platform, go.mod, router, routeTree), keep the change minimal and rebase right
   before opening the PR.
2. Never edit `platform/*` (Phase A owns it). Need something there? Write the
   need in your feature's progress file and build a small private helper in your
   feature until Phase A/owner adds it.
3. Do NOT edit `docs/maintainers/migration-progress.md`, `ROADMAP.md` or the
   shared progress table. Only your per-feature file.
4. goose migrations: reserve your own number range to avoid clashes. Lane 1 uses
   00100-00199, lane 2 uses 00200-00299, and so on (lane N: N00-N99 prefixed `00`).
   New tables only, prefixed `go_`. Never alter legacy tables.
5. Generated files (`routeTree.gen.ts`, OpenAPI output): never hand-edit. After
   rebase, regenerate. `go.mod`/`go.sum`: after rebase run `go mod tidy`.
6. Rebase on `main` before opening each PR. Resolve conflicts, rerun checks.

### Port rules

- Handler -> service -> repository. Stdlib first (`net/http` ServeMux patterns,
  `slices`, `maps`, `errors`, `slog`, `errgroup`). pgx + plain SQL. No ORM, no
  framework, no getters/setters, no forwarding wrappers, no comments that repeat
  the code, no dead code. Use platform helpers (`httpx`, `pgdb`, `jobs`,
  `dataforseo`, `entitlements`) instead of writing your own.
- Existing legacy tables are read/written with plain SQL, schema unchanged.
- Auth: reuse `backend/internal/auth` (session cookie, project membership). Every
  query tenant-scoped. Each repository gets a test proving org A cannot read org B.
- Validate input in the handler, JSON errors via `httpx`, paginate lists, SSRF
  guard on any user-supplied URL, timeout on every outbound call.
- Parity: port the legacy TS tests of your module to Go (same inputs, same expected
  outputs, edge cases: empty, unicode, max size, API errors, timeouts). External
  APIs are faked with `httptest` using recorded response shapes. No parity test =
  not done.
- Frontend per mini-task: replace the matching `createServerFn` calls with the
  generated typed client + TanStack Query, keep UI behavior identical, delete the
  dead TS server code in the same PR (knip must stay green).
- Plan limits: before creating a billable resource call
  `entitlements.Check(ctx, orgID, resource)` from platform.
- No Docker on the owner's machine. Run locally what works (gofmt, go vet, go build,
  staticcheck, DB-free tests, `pnpm tsc`, oxlint, prettier). GitHub CI runs the
  full suite with Postgres + Redis. Read CI, fix failures on your branch.

### Done criteria per PR

Go: gofmt, go vet, staticcheck, golangci-lint, `go test -race -cover` (CI),
govulncheck, `go mod tidy`, deadcode. Frontend: `pnpm ci:check` and `pnpm test`.
PR description: what moved, TS files deleted, net lines (TS deleted vs Go added),
parity test list, commands run + results, risks, new dependencies with reason.

---

## PHASE A (run ALONE first, ~3-4 h, then merge before lanes start)

Single worker. Branch `mig/foundation`. Do these as separate small PRs in order
(A1 first, merge; others may follow quickly):

A1. Clean item 0 on its own branch (not `main`): move `writeJSON`/`writeError`/`apiError`
into `platform/httpx` (one copy); delete dead code in `httpapi/` (billingStatus,
billingCheckout, billingUser, razorpayWebhook, collect wrappers, validateCollect,
collectRequest, clientIP, publicOrigin, maxWebhookBody, type userKey); mount
feature handlers directly in the router via each feature's `Mount`; move the
tests of deleted code into the feature packages.
A2. `platform/httpx`: error envelope, JSON decode with size limit, pagination
(`page`,`limit` parsing), request-id + logging middleware.
A3. `platform/pgdb`: pool helper, `InTx` helper, test helper for a real Postgres.
A4. `platform/jobs`: Postgres-backed queue (enqueue, claim with `FOR UPDATE SKIP
    LOCKED`, retry with backoff, timeout, idempotency key) + worker runner.
A5. `platform/dataforseo`: one client (auth, retries with backoff, timeout, per-org
cost tracking) behind a small interface. Port from the TS DataForSEO code.
A6. `platform/entitlements`: `Check(ctx, orgID, resource) error` with an
allow-all implementation for now (lane 9 implements the real limits).
A7. OpenAPI: per-feature specs under `backend/api/*.yaml` merged by one codegen
command into Go server types and a typed TS client; CI fails when generated
code is stale. Document the `Mount` convention with the analytics feature as
the reference example.
A8. `docs/maintainers/migration/README.md`: lane list, ownership, rules above, and
a per-feature progress template. Replace the single shared progress table.

Gate: Phase A is done when A1-A8 are merged. Lanes may start reading code and
recording parity fixtures before that, but must not merge before A1 and A2.

---

## LANE 1: identity (migration range 00100)

Features: organization, workspace, onboarding, config, projects, projectContext.
TS sources: `src/serverFunctions/{organization,workspace,onboarding,config,projects,projectContext}.ts`, `src/server/features/{projects,project-context}`, `src/server/auth`.

- M1 organizations + workspace read endpoints (current org, members, roles).
- M2 projects CRUD (tenant-scoped, membership checked).
- M3 project-context (read/write, parity with TS).
- M4 onboarding + config endpoints; remove the matching TS.

## LANE 2: keywords (range 00200)

Features: domain, serp-locations, keywords.
TS sources: `src/serverFunctions/{domain,serp-locations,keywords}.ts`, `src/server/features/{domain,keywords}`.

- M1 serp-locations incl. India city-level locations (cacheable, paginated).
- M2 domain overview (uses `platform/dataforseo`).
- M3 keyword research (ideas, volumes, difficulty) with cost tracking.
- M4 saved keywords and clustering.

## LANE 3: rank tracking (range 00300)

Features: rank-tracking and its workflows.
TS sources: `src/serverFunctions/rank-tracking.ts`, `src/server/features/rank-tracking`, `src/server/workflows`.

- M1 tracked keywords + location config CRUD.
- M2 scheduled check job on `platform/jobs` (idempotent, retries).
- M3 history, trends and competitor positions API.
- M4 frontend switch and TS delete.

## LANE 4: site audit (range 00400)

Features: audit, lighthouse, crawlerAccess.
TS sources: `src/serverFunctions/{audit,lighthouse,crawlerAccess}.ts`, `src/server/features/audit`, audit workflows.

- M1 start/status API and result storage.
- M2 crawler job with SSRF guard and robots handling.
- M3 lighthouse runs.
- M4 paginated results API, frontend switch, TS delete.

## LANE 5: backlinks and assistants (range 00500)

Features: backlinks, ahrefs adapters, ai-search, sam, samAccess.
TS sources: `src/serverFunctions/{backlinks,ahrefs,ai-search,sam,samAccess}.ts`, `src/server/features/{backlinks,ai-search,sam}`.

- M1 backlinks overview, referring domains, anchors.
- M2 ahrefs adapters (only if still used; else delete and record why).
- M3 ai-search (wait for Lane 10 M1 `platform/llm` before using an LLM client).
- M4 sam + samAccess.

## LANE 6: reports and branding (range 00600)

Features: reports, reportTemplates, branding (white-label).
TS sources: `src/serverFunctions/{reports,reportTemplates,branding}.ts`, `src/server/features/{reports,branding}`.

- M1 branding (logo, name, color, website) incl. image upload validation.
- M2 report templates.
- M3 reports, share links, PDF/export.
- M4 frontend switch and TS delete.

## LANE 7: Google data (range 00700)

Features: ga4, gsc, googleAccounts, searchPerformance.
TS sources: `src/serverFunctions/{ga4,gsc,googleAccounts,searchPerformance}.ts`, `src/server/features/{ga4,gsc,google}`.

- M1 `platform/google` is NOT in Phase A: you own a `google` feature package for OAuth,
  encrypted token storage and refresh.
- M2 GA4 reports.
- M3 GSC reports.
- M4 searchPerformance and frontend switch.

## LANE 8: growth and admin (range 00800)

Features: dashboard, activation, referrals, gdpr, email.
TS sources: `src/serverFunctions/{dashboard,onboarding?}.ts` (dashboard only), `src/server/features/{dashboard,activation}`, `src/server/{referrals,gdpr,email}`.

- M1 dashboard summary endpoint (aggregates through other features' public services; do not import their repositories).
- M2 activation checklist.
- M3 referrals.
- M4 gdpr export/delete and email sending behind a `Mailer` interface with a log implementation for dev.

## LANE 9: billing (range 00900) = ROADMAP 5.3

Features: billing remnants in TS + live usage + one-click cancel.
TS sources: `src/serverFunctions/billing.ts`, `src/server/billing`.

- M1 plan limits defined once in Go; real `entitlements` implementation replacing the allow-all stub.
- M2 usage API (`GET /api/v1/billing/usage`) and 402 `{code:"plan_limit",limit,used}` responses.
- M3 cancel at period end (`POST /api/v1/billing/cancel`), idempotent, audit row.
- M4 billing UI: usage meters (80% amber, 100% red), one confirm dialog to cancel, "active until <date>".
- M5 delete the TS billing code.

## LANE 10: AI visibility (range 01000) = ROADMAP 2.4 + 2.5

New feature (not a port).

- M1 `platform/llm`: one provider interface with OpenAI, Perplexity, Gemini and Anthropic clients (timeout, retry with backoff, token/cost tracking per org, a provider without a key is skipped). Merge this first.
- M2 data model (`go_ai_*`, normalized): brands, competitors, prompts, runs, answers, mentions, citations. CRUD API.
- M3 daily run worker on `platform/jobs` (idempotent, respects `entitlements`).
- M4 extraction: mentions + position, citation URLs, sentiment via a cheap LLM call with strict JSON validated in Go.
- M5 summary API: score, share of voice, trend, top cited domains, per-provider breakdown, drop alert email (>20% week over week).
- M6 UI: setup wizard under 5 minutes, score card, charts, citations table, per-prompt answer view with brand highlighted.
- M7 join with analytics: "seen in AI" vs "visitors from AI" (ROADMAP 2.5).

## LANE 11: MCP and cleanup (start ONLY after lanes 1-8 are merged)

- M1 MCP server in Go (`src/server/mcp`), same tools and schemas.
- M2 delete `src/server`, `src/serverFunctions`, Cloudflare/Alchemy/wrangler config and unused deps.
- M3 Go serves the Vite build (SPA fallback, cache headers). ROADMAP 0.4.
- M4 update README, docs, ROADMAP.

---

## Reviewer loop

Send each PR link to the reviewer (Claude). Fix every finding on the same branch
before opening that lane's next PR. Do not merge your own PRs.
