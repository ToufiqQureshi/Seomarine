# Codex task 2: take the migration to 50%

Owner: Codex (all rows, including the ones Buffy had). Reviewer and merger: Claude.
Read first: `STATUS.md`, `FEATURES.md`, `README.md`, `AGENTS.md`, `CLAUDE.md`.

## Goal

Move TS backend lines to Go **and delete the TS**. Target: **about 24,000 of the
47,706 TS lines gone** (50%). Progress is measured only by this number going down:

```sh
find src/server src/serverFunctions src/db -name "*.ts" ! -name "*.test.ts" | xargs cat | wc -l
```

Go written without deleting TS does not count. No feature may be removed or
broken: same behavior, same data, same UI.

## Rules (non-negotiable)

1. **Parity tests first.** Port the TS tests to Go (`httptest` for handlers,
   real Postgres for repositories, no mocked SQL). A TS file is deleted only when
   its Go parity tests pass. Never weaken, skip or delete a test to get green.
2. **GitHub's Linux CI is the judge.** Local Windows cannot run Go tests (policy
   blocks the test exe). Run `gofmt`, `go vet`, and `pnpm ci:check` locally, push,
   and read the CI output. Do not call work done until `checks`, `ci` and
   `docker-build` are all green on the PR.
3. **Features import each other's TS**, so one feature cannot always be deleted
   alone. Bundle the clusters below into one PR so the PR can delete the TS and
   stay green. If a TS file still has an outside consumer, leave it and record the
   consumer in the feature's progress file.
4. **Frontend switch in the same PR.** The React call moves from the server
   function to `apiRequest("/api/v1/...")` (it takes an optional JSON body since
   the branding PR). Delete the server function.
5. **Routes, not just functions.** `FEATURES.md` lists 10 server routes under
   `src/routes` (OAuth callbacks, `api/auth`, `api/autumn`, public report share).
   They must be ported before their feature is called DONE.
6. **Keep records honest.** In the same PR: update `STATUS.md` (columns Go and TS
   as in `mig/migration-status-split`), the feature's progress file, and tick the
   rows in `FEATURES.md`. Never delete a row; use `DROPPED` plus a reason only if
   the owner approves.
7. **Quality gates** are in `AGENTS.md`. `deadcode -test` is now the CI rule, so
   shared platform code must be called by a feature or a test.
8. One PR per cluster. Push the branch and open the PR. Claude reviews and merges;
   do not merge yourself.

## Step 0: finish PR #19 (branding)

`mig/branding` is behind `main` and red. Merge `main` into it, then:

- `backend/internal/httpapi/httpapi.go` conflicts: billing now uses
  `billing.Mount(mux, billing.Deps{...})`; give `branding` the same shape
  (`branding.Deps{Logger, Service, WithSession}`).
- Auth context moved: use `auth.UserFromContext` / `auth.WithUser`, not `httpx`.
- staticcheck reported ST1005 on error strings in `internal/branding/handler.go`;
  reproduce with `staticcheck ./internal/branding/...` and fix the cause.
- `pnpm exec prettier --write docs/maintainers/migration/branding.md`.
- Add `organization_branding` to `backend/internal/database/testdata/legacy_schema.sql`.
- Answer in the PR: does the SVG logo (data URL) get rendered server-side
  anywhere? If yes, that is an injection risk and must be sanitized.

## Order and bundles

Do them in this order (dependencies first). Line counts are TS lines removed once
the cluster is fully deleted; the running total is the plan to ~50%.

| #   | Cluster (one PR each)                                                     | TS lines | Running total |
| --- | ------------------------------------------------------------------------- | -------: | ------------: |
| 1   | email, ai-search, backlinks + ahrefs                                      |    2,760 |         2,760 |
| 2   | domain, project-context, platform-lib leftovers                           |    2,684 |         5,444 |
| 3   | keywords (+ serp-locations)                                               |    2,201 |         7,645 |
| 4   | google, ga4, gsc (OAuth callbacks `api/ga4`, `api/gsc` included)          |    4,688 |        12,333 |
| 5   | rank-tracking                                                             |    3,343 |        15,676 |
| 6   | platform-dataforseo (the 24 TS files; Go client already merged)           |    4,692 |        20,368 |
| 7   | identity (organizations, workspace, onboarding, config; later `api/auth`) |    1,349 |        21,717 |
| 8   | reports + branding (flips branding DONE; routes `r/`, `s/`)               |    1,686 |        23,403 |
| 9   | sam                                                                       |    1,964 |        25,367 |

Later (not part of this target, do not start early): dashboard + activation +
referrals, audit, billing (29 consumers), mcp (needs activation), and **gdpr
last** (depends on google, sam, referrals, audit, KV, R2 and Durable Objects).
`db-schema` (3,093) can only be deleted at the very end.

If a cluster turns out bigger or riskier than the table says, stop, write what
you found in its progress file, and move to the next one. Do not ship a half
cluster that leaves `main` red.

## Report format after each PR

- PR number and CI status (all three checks).
- TS lines before and after (the command above).
- Which TS files were deleted and which still have a consumer, and who.
- Anything you could not port, and why.

## Infra items you may do between clusters

- Shared pgdb test helper, pool tuning.
- Rebase the old local `mig/l1-identity-m1` work (organizations) onto `main`
  when you reach cluster 7.
- Check whether the legacy TS app (Cloudflare Workers) can run on Railway at all;
  report it in `docs/maintainers/migration/README.md`. If not, the strangler proxy
  needs another host and the owner decides.
