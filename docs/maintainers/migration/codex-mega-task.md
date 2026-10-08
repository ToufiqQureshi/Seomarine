# CODEX MEGA TASK: migrate 40 items from TypeScript to Go

Paste this whole file into the Codex session. Read `parallel-tasks.md` COMMON section
too (same rules apply, this file only adds the 40-item order and the safety rules).

## Goal

Move ALL backend logic from `src/server`, `src/serverFunctions`, `src/db` (TypeScript,
~47k lines) to Go in `backend/`. React/TS stays only as frontend calling `/api/v1/*`.
Delete the TS code of each item in the SAME PR that replaces it. Never break a feature.

## Hard safety rules (production must keep working after EVERY PR)

1. `main` must be green before you start an item and after each merge. If `main` is
   red, fix that first (smallest possible PR). Never leave CI red.
2. One item = one branch `mig/<lane>-<item>` from latest `main` = one PR (<~800 changed
   lines excluding deleted TS and generated code). Open the PR, wait for CI, fix
   failures on the same branch, then continue. If CI is red for the same cause twice,
   stop and write the blocker in `docs/maintainers/migration/<feature>.md`.
3. Strangler order inside an item: (a) write Go endpoint + parity tests, (b) switch the
   frontend call to the generated typed client, (c) delete the dead TS server code
   and its tests. If (a) parity is not proven, do NOT do (b) or (c).
4. Parity proof is mandatory: port the legacy TS tests to Go with identical inputs and
   expected outputs, plus edge cases (empty, unicode, max size, API error, timeout).
   External APIs faked with `httptest` using recorded response shapes. No test
   skipped, weakened or deleted to get green.
5. Never change legacy table schemas or data. New tables only, prefixed `go_`, goose
   migrations in your lane range (lane N uses N00-N99, prefixed `00`). Migrations must
   be backward compatible (old TS and new Go run side by side during the migration).
6. Security on every endpoint: session auth via `internal/auth`, tenant-scoped queries
   (each repository gets a test "org A cannot read org B"), SSRF guard on any
   user-supplied URL, timeout on every outbound call, input validated in handler,
   paginated lists, no secrets in logs or commits.
7. Do not edit `platform/*` outside Phase A items. Do not edit ROADMAP.md or the shared
   progress file. Per-feature progress goes in `docs/maintainers/migration/<feature>.md`.
8. Brand is Seomarine only. Never add "OpenSEO". Never touch `LICENSE-OPENSEO-MIT`.
9. No Docker on the owner's machine. Locally run: gofmt, go vet, go build, staticcheck,
   DB-free tests, `pnpm tsc`, oxlint, prettier, knip. GitHub CI runs the rest.
   Windows blocks test exes in %TEMP%: use `GOTMPDIR=D:\gotmp` for `go test`.
10. Style: handler -> service -> repository, stdlib first, pgx + plain SQL, no ORM, no
    wrappers, no dead code (CI `deadcode` must report nothing), no comments repeating
    code. Each feature has `Mount(mux, deps)` registered from the root router.

## Per-PR done criteria

gofmt, go vet, staticcheck, golangci-lint, `go test -race`, govulncheck, `go mod tidy`,
deadcode clean; frontend `pnpm ci:check` and `pnpm test` green. PR body: what moved, TS
files deleted, net lines (TS deleted vs Go added), parity test list, commands run +
results, risks, new dependencies with reason. Do not merge your own PRs: the reviewer
(Claude) reviews each PR; fix every finding on the same branch before the next item.

## The 40 items (do in this order; blockers noted)

### Phase A leftovers (platform, must exist before the features)

1. A3 `platform/pgdb` (pool, InTx, test DB helper). PR for it may already exist: finish it.
2. A4 `platform/jobs` (Postgres queue, SKIP LOCKED, retries, timeout, idempotency, runner).
3. A5 `platform/dataforseo` (auth, backoff, timeout, per-org cost tracking).
4. A6 `platform/entitlements` (`Check(ctx, orgID, resource)`, allow-all stub).
5. A7 OpenAPI per feature + codegen (Go types + typed TS client), CI fails on stale code.
   Also fix review leftovers on `platform/httpx`: remove the `auth` import (platform must
   not depend on features), add panic-recovery middleware, log response status code,
   make `Mount` signature `Mount(mux, Deps)` and unexport handler funcs not needed outside.

### Lane 1 identity (migrations 00100+)

6. organizations + workspace read endpoints (a draft exists in worktree `wt-l1`,
   branch `mig/l1-identity-m1`, rebase it on main).
7. projects CRUD.
8. project-context read/write.
9. onboarding + config.

### Lane 2 keywords (00200+)

10. serp-locations incl. India city-level.
11. domain overview.
12. keyword research with cost tracking.
13. saved keywords + clustering.

### Lane 3 rank tracking (00300+)

14. tracked keywords + location config CRUD.
15. scheduled check job on `platform/jobs`.
16. history, trends, competitor positions.
17. frontend switch + TS delete for rank-tracking and its workflows.

### Lane 4 site audit (00400+)

18. audit start/status + result storage.
19. crawler job (SSRF guard, robots.txt).
20. lighthouse runs.
21. paginated results API, frontend switch, TS delete (audit, lighthouse, crawlerAccess).

### Lane 5 backlinks and assistants (00500+)

22. backlinks overview, referring domains, anchors.
23. ahrefs adapters (only if still used; else delete and record why).
24. ai-search (needs an LLM client: build a minimal private one, no `platform/llm` yet).
25. sam + samAccess.

### Lane 6 reports and branding (00600+)

26. branding (logo/name/color/website, validate image upload).
27. report templates.
28. reports + share links + PDF/export.
29. frontend switch + TS delete for reports/branding.

### Lane 7 Google data (00700+)

30. `google` package: OAuth, encrypted token storage, refresh (googleAccounts).
31. GA4 reports.
32. GSC reports.
33. searchPerformance + frontend switch.

### Lane 8 growth and admin (00800+)

34. dashboard summary (use other features' services, never their repositories).
35. activation checklist.
36. referrals.
37. gdpr export/delete + email behind a `Mailer` interface (log impl for dev).

### Lane 9 billing (00900+)

38. real plan limits + `entitlements` implementation + usage API
    (`GET /api/v1/billing/usage`, 402 `{code:"plan_limit",limit,used}`).
39. cancel at period end (`POST /api/v1/billing/cancel`, idempotent, audit row) +
    delete remaining TS billing code.

### Final (only after items 1-39 are merged)

40. MCP server in Go (same tools and schemas), delete `src/server`, `src/serverFunctions`,
    `src/db` server usage, Cloudflare/Alchemy/wrangler config and unused deps, make Go
    serve the Vite build (SPA fallback, cache headers). Update README and docs.

## Working loop

For each item: sync `main` -> branch -> read legacy TS -> Go code + parity tests ->
frontend switch -> delete TS -> run all local checks -> rebase -> open PR -> wait for CI ->
fix -> report PR link to the reviewer -> next item. If an item is bigger than ~800 lines,
split it (item 12a, 12b, ...) and say so in the PR. Stop and ask only when blocked by a
decision the owner must make; never skip an item silently.
