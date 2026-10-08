# Handoff: Buffy lane — status, blocker, recommendation

Audience: Claude Code (reviewer / next agent). Written 2026-10-08 on branch
`mig/branding`, base `main` @ `71aa7fc`.

## 1. Shipped

Branch `mig/branding` (pushed to origin, no `main` push):

- `backend/internal/branding/` — Go port of white-label report branding:
  handler, service, repository (plain SQL, `organization_branding` table
  unchanged), types.
- `backend/api/branding.yaml` — OpenAPI: `GET /api/v1/branding`,
  `POST /api/v1/branding`, `POST /api/v1/branding/reset`.
- `backend/internal/branding/branding_test.go` — parity tests for the zod
  schema semantics plus a round-trip and an org-isolation test.
- `src/client/features/branding/brandingApi.ts` (+ test) — settings page calls
  the Go API through `apiRequest`.
- `src/client/lib/seomarineApi.ts` — `apiRequest` now accepts an optional JSON
  body (additive; GET/POST-only before).
- `src/serverFunctions/branding.ts` — **deleted**.
- `docs/maintainers/migration/branding.md` — per-feature progress.

Verified on the branch: `gofmt -l` clean, `go vet ./...` clean,
`go test ./...` green (13 packages), `pnpm tsc --noEmit` clean, `pnpm knip`
clean, branding vitest 19/19. Full `pnpm test` failures are identical on
untouched `main` (google OAuth, string-retention, samSkills — Windows temp/env
issues), so this branch adds none.

The `branding` STATUS row was **not** flipped to DONE. Reason in §2.

## 2. The blocker: the lane model in `parallel-tasks.md` does not hold

`parallel-tasks.md` assumes: one feature per branch, lanes independent, and
"delete the dead TS server code in the same PR". In this codebase features
import each other's TypeScript, so a feature cannot be deleted in isolation and
its row cannot reach DONE on its own.

Measured outside-consumers per feature (non-test files importing
`@/server/<feature>/`, `@/server/features/<feature>/` or `@/server/lib/<feature>`):

| Feature                      | Outside consumers | Notable consumers                                                   |
| ---------------------------- | ----------------: | ------------------------------------------------------------------- |
| billing                      |                29 | plan limits across the app                                          |
| audit                        |                14 | dashboard, reports                                                  |
| reports                      |                10 | branding, report routes                                             |
| google                       |                 9 | ga4, gsc                                                            |
| gsc                          |                 6 | dashboard                                                           |
| activation                   |                 5 | dashboard, **mcp (Codex lane)**                                     |
| referrals                    |                 5 | dashboard                                                           |
| ga4                          |                 4 | dashboard                                                           |
| branding                     |                 3 | reports + `src/routes/r/$reportId.ts`, `src/routes/s/$token/raw.ts` |
| sam / backlinks / email      |                 3 | —                                                                   |
| ai-search / dashboard / gdpr |                 1 | entry points                                                        |

Concrete consequences:

- **branding → reports.** `BrandingService.getBranding` is called by
  `src/routes/r/$reportId.ts`, `src/routes/s/$token/raw.ts` and
  `reports/sharePage.tsx`. Deleting `BrandingService.ts` /
  `BrandingRepository.ts` / `brandBar.tsx` now breaks `tsc`/knip. The branding
  row must flip to DONE inside the `reports` PR.
- **activation → dashboard _and_ mcp.** `ActivationRepository` is imported by
  `DashboardService.ts` and `serverFunctions/dashboard.ts`; `mcpActivation.ts`
  is imported by `src/server/mcp/{instrumentation,oauth-provider,api-key-auth}.ts`.
  Buffy therefore cannot finish `activation` before Codex lands `mcp`
  (Wave 2). `workspace-merge.ts` (Codex, identity) also writes
  `organization_activation_state`.
- **`gdpr` is not a leaf.** `src/server/gdpr/storage-erasure.ts` depends on
  google OAuth, sam (`SamChatAgent`), referrals (`dub`), audit + rank
  workflows, KV, R2 and Durable Objects. It is one of the last things that can
  move, not an early one.
- **`email` is not a leaf either.** `src/lib/auth.ts`,
  `src/serverFunctions/organization.ts` and `src/server/billing/loops-sync.ts`
  import it.

Note: counting consumers is not the same as dependency depth. `gdpr` has one
consumer but depends on almost the whole app; ordering must follow
dependencies, not consumer counts.

## 3. Recommendation

The current single "Status" column cannot express the true state of a feature
("Go written, TS still live because a consumer hasn't moved"). Every honest
option needs that distinction:

1. **Add a second column to STATUS.md**, e.g. `Go` (`TODO` / `DONE`) and `TS`
   (`LIVE` / `DELETED`), and keep DONE only for "Go done AND TS deleted".
   Without this, progress is unmeasurable and rows get marked DONE dishonestly.
2. **Migrate bottom-up in dependency order**, accepting that early PRs add Go
   without deleting TS:
   `branding/email/ai-search` (Go-only) → `backlinks + ahrefs` → `ga4` →
   `gsc` → `google` → `reports` (flips branding DONE) → `sam` →
   `dashboard + activation + referrals` → `audit` → `billing` → and `gdpr`
   last.
3. **Bundle the clusters that share consumers into one PR** where a row must
   actually reach DONE: `reports + branding`, `dashboard + activation +
referrals`, and `mcp + activation` (cross-lane, needs Codex).

I would take 1 + 3: it keeps PRs honest, lets DONE mean something, and is the
only way rows can move without shipping red.

## 4. Concrete asks for the reviewer

1. Review `mig/branding` — especially the API shape. `POST /branding` and
   `POST /branding/reset` were chosen over `PUT`/`DELETE` because
   `apiRequest` speaks GET and POST only and the legacy functions were POSTs;
   confirm or change.
2. Confirm that extending `src/client/lib/seomarineApi.ts` with an optional
   JSON body is acceptable as a shared-platform change (it is additive, but it
   is outside the feature folder).
3. `backend/internal/database/testdata/legacy_schema.sql` has no
   `organization_branding` table; the branding tests create it inline. Decide
   whether it should be added to the shared fixture.
4. Decide policy for §3.1 (extra STATUS column) and §3.3 (bundling), since
   that changes how every remaining row is delivered.

## 5. Where this stopped

Worktree `D:\seomarine\wt-buffy`, branch `mig/branding`, working tree clean.
Remaining in the lane: 15 features, ~20,000 TS lines, plus the parity tests,
frontend switches and the docs rewrite.
