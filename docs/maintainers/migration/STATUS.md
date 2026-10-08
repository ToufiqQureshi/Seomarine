# Seomarine: TypeScript -> Go migration status

Source of truth for the migration. Measured on `main` @ `f8cefcd` (2026-10-08).

**Target architecture:** React/TS = frontend UI only (`src/client`). ALL backend = Go (`backend/`). No feature is removed: same behavior, same data, same UI.

## How this file works

- Every row below is a TS server file that must end up as Go and then be **deleted**.
- A file moves to Go in a PR. In the SAME PR its row is set to `DONE` and the TS file is deleted. The totals in "Summary" must go DOWN. Never mark DONE without Go parity tests passing.
- Recount at any time (this number must only ever shrink):
  `find src/server src/serverFunctions src/db -name "*.ts" ! -name "*.test.ts" | xargs cat | wc -l`
- Owners: **Codex** = platform + core features, **Buffy** = growth/data/integration features. Reviewer: Claude. Do not edit rows of the other owner.

## Summary

> Line counts are TS lines. The Go port is expected to be smaller (standard library and pgx replace TS glue). Progress = TS lines deleted, not Go lines written.

**TS backend left: 296 files, 43,806 lines** (plus test files to port to Go tests, then delete). Measured with the command above; historical per-feature estimates below are not yet recounted. Go today: ~3,000 lines.

| Feature             | TS files |   TS lines | Test files | Go package                                         | Owner | Wave | Status |
| ------------------- | -------: | ---------: | ---------: | -------------------------------------------------- | ----- | ---- | ------ |
| audit               |       40 |      6,781 |         24 | audit                                              | Buffy | 1    | TODO   |
| ga4                 |       14 |      2,773 |          7 | ga4                                                | Buffy | 1    | TODO   |
| sam                 |        9 |      1,964 |          4 | sam                                                | Buffy | 1    | TODO   |
| reports             |        9 |      1,499 |          7 | reports                                            | Buffy | 1    | TODO   |
| billing             |       10 |      1,407 |          5 | billing                                            | Buffy | 1    | TODO   |
| ai-search           |        6 |      1,303 |          5 | ai-search                                          | Buffy | 1    | TODO   |
| gsc                 |        8 |      1,234 |          4 | gsc                                                | Buffy | 1    | TODO   |
| backlinks           |        7 |      1,022 |          2 | backlinks                                          | Buffy | 1    | TODO   |
| google              |        7 |        681 |          5 | google                                             | Buffy | 1    | TODO   |
| dashboard           |        3 |        381 |          0 | dashboard                                          | Buffy | 1    | TODO   |
| referrals           |        2 |        355 |          1 | referrals                                          | Buffy | 1    | TODO   |
| email               |        2 |        316 |          0 | email (Mailer)                                     | Buffy | 1    | TODO   |
| gdpr                |        1 |        297 |          0 | gdpr                                               | Buffy | 1    | TODO   |
| activation          |        2 |        226 |          1 | activation                                         | Buffy | 1    | TODO   |
| branding            |        4 |        187 |          1 | branding                                           | Buffy | 1    | TODO   |
| ahrefs              |        1 |        119 |          0 | backlinks (adapter)                                | Buffy | 1    | TODO   |
| mcp                 |       47 |      9,216 |         28 | mcp                                                | Codex | 2    | TODO   |
| platform-dataforseo |       24 |      4,692 |         15 | platform/dataforseo                                | Codex | 1    | TODO   |
| rank-tracking       |       12 |      3,343 |          8 | rank-tracking                                      | Codex | 1    | TODO   |
| db-schema           |       33 |      3,093 |          2 | (deleted at the end; Go uses plain SQL)            | Codex | 2    | TODO   |
| keywords            |       14 |      2,201 |          5 | keywords                                           | Codex | 1    | TODO   |
| identity            |       15 |      1,349 |          4 | organizations, workspace, onboarding, config, auth | Codex | 1    | TODO   |
| platform-lib        |       14 |        937 |          4 | platform/\* (shared helpers)                       | Codex | 1    | TODO   |
| project-context     |        4 |        897 |          2 | project-context                                    | Codex | 1    | TODO   |
| domain              |        7 |        850 |          2 | domain                                             | Codex | 1    | TODO   |
| projects            |        4 |        554 |          1 | projects                                           | Codex | 1    | TODO   |
| workflows           |        1 |         29 |          0 | platform/jobs + owning feature                     | Codex | 1    | TODO   |
| **TOTAL**           |  **300** | **47,706** |    **137** |                                                    |       |      |        |

Split: Codex 27,161 lines, Buffy 20,545 lines.

## Goal for today: 60% of the app in Go

60% of 47,706 lines = **28,623 lines** deleted from TS. Wave 1 (everything except MCP and the db schema, which can only go last) is 35,397 lines = **74%**, so the 60% target needs about 80% of Wave 1 done. MCP (9,216 lines) is Wave 2 and goes last.

**Honest warning:** this much in one day is only possible if Codex and Buffy run non-stop in parallel, CI is fast and review does not block. Every PR still needs green CI and parity tests, because a broken production is worse than a late migration. If something must give, it is speed, never the parity tests.

Order (biggest wins first, platform before features):

1. Codex: Phase A leftovers (jobs, dataforseo, entitlements, OpenAPI codegen, httpx fixes), then `platform-lib`, then identity -> projects -> project-context -> domain -> keywords -> rank-tracking -> workflows.
2. Buffy (starts immediately; needs only `httpx` + `pgdb`, both merged): audit (biggest, 6.8k), billing, ga4, gsc, google, backlinks, ai-search, sam, reports, branding, dashboard, activation, referrals, gdpr, email.
3. Wave 2 (after all above): mcp, then delete `db-schema` (src/db, only when no TS query is left), Cloudflare/Alchemy/wrangler config, and make Go serve the React build.

Rules for every PR are in `TASK-CODEX-2.md`. Features that need `platform/jobs`, `dataforseo` or `entitlements` before Codex has merged them: build a small private helper in the feature and note it in `docs/maintainers/migration/<feature>.md`.

## Docs that are outdated (must be rewritten for the Go backend)

`CLAUDE.md`, `AGENTS.md`, `README.md`, `ROADMAP.md`, `docs/LOCAL_DEVELOPMENT.md`, `docs/maintainers/*` (including `README.md`, `review-guidelines.md`, `releases.md`), `docs/SELF_HOSTING_*.md`. Owner: Buffy. Each feature PR updates the docs it made wrong; one final full rewrite after Wave 2. Rule: a doc may only describe things that exist on `main`.

## Files: Codex

Codex file-level progress is kept in the feature notes below. Do not mark a feature DONE until its Go route, frontend integration, parity tests, and TypeScript deletion are complete.

- [mcp](mcp.md) — 47 files, 9,216 lines; TODO
- [platform-dataforseo](platform-dataforseo.md) — 24 files, 4,692 lines; TODO
- [rank-tracking](rank-tracking.md) — 12 files, 3,343 lines; TODO
- [db-schema](db-schema.md) — 33 files, 3,093 lines; TODO
- [keywords](keywords.md) — 14 files, 2,201 lines; TODO
- [identity](identity.md) — 15 files, 1,349 lines; TODO
- [platform-lib](platform-lib.md) — 14 files, 937 lines; TODO
- [project-context](project-context.md) — 4 files, 897 lines; TODO
- [domain](domain.md) — 7 files, 850 lines; TODO
- [projects](projects.md) — 4 files, 554 lines; TODO
- [workflows](workflows.md) — 1 files, 29 lines; TODO

## Files: Buffy

### audit (40 files, 6,781 lines) -> `backend/internal/audit`

| TS file (src/)                                                      | Lines | Status |
| ------------------------------------------------------------------- | ----: | ------ |
| `server/workflows/siteAuditWorkflowCrawl.ts`                        |   482 | TODO   |
| `server/workflows/siteAuditWorkflowPhases.ts`                       |   465 | TODO   |
| `server/features/audit/repositories/AuditRepository.ts`             |   431 | TODO   |
| `server/lib/audit/discovery.ts`                                     |   376 | TODO   |
| `server/features/audit/AuditScratchpad.ts`                          |   356 | TODO   |
| `server/features/audit/services/AuditService.ts`                    |   351 | TODO   |
| `server/workflows/site-audit-workflow-helpers.ts`                   |   313 | TODO   |
| `server/lib/audit/url-policy.ts`                                    |   297 | TODO   |
| `server/lib/audit/page-analyzer.ts`                                 |   281 | TODO   |
| `server/lib/lighthouseStoredPayload.ts`                             |   272 | TODO   |
| `server/lib/audit/rendered-page.ts`                                 |   203 | TODO   |
| `server/features/audit/services/CrawlerCredentialService.ts`        |   186 | TODO   |
| `server/lib/audit/issues/multipage-checks.ts`                       |   181 | TODO   |
| `server/lib/audit/types.ts`                                         |   178 | TODO   |
| `server/features/audit/services/auditReconciler.ts`                 |   176 | TODO   |
| `server/lib/audit/lighthouse.ts`                                    |   169 | TODO   |
| `server/workflows/SiteAuditWorkflow.ts`                             |   169 | TODO   |
| `server/lib/audit/issues/page-reporters.ts`                         |   166 | TODO   |
| `server/lib/audit/rendering-billing.ts`                             |   162 | TODO   |
| `server/lib/audit/url-utils.ts`                                     |   158 | TODO   |
| `server/lib/lighthousePayload.ts`                                   |   123 | TODO   |
| `server/lib/audit/crawl-throttle.ts`                                |   116 | TODO   |
| `server/lib/audit/crawl-window.ts`                                  |   110 | TODO   |
| `server/features/audit/scratchpad-sql.ts`                           |   103 | TODO   |
| `server/features/audit/repositories/CrawlerCredentialRepository.ts` |    98 | TODO   |
| `server/features/audit/services/shopifySignature.ts`                |    96 | TODO   |
| `serverFunctions/audit.ts`                                          |    95 | TODO   |
| `serverFunctions/lighthouse.ts`                                     |    88 | TODO   |
| `server/lib/audit/progress-kv.ts`                                   |    80 | TODO   |
| `server/lib/audit/audit-errors.ts`                                  |    79 | TODO   |
| `server/features/audit/services/audit-capacity.ts`                  |    65 | TODO   |
| `server/lib/audit/issues/multipage.ts`                              |    62 | TODO   |
| `server/workflows/auditStepConfigs.ts`                              |    58 | TODO   |
| `server/lib/audit/html-response.ts`                                 |    49 | TODO   |
| `serverFunctions/crawlerAccess.ts`                                  |    47 | TODO   |
| `server/features/audit/repositories/auditIssueWrites.ts`            |    45 | TODO   |
| `server/lib/audit/classify-fetch.ts`                                |    37 | TODO   |
| `server/lib/audit/ids.ts`                                           |    24 | TODO   |
| `server/features/audit/repositories/auditSummaryQueries.ts`         |    21 | TODO   |
| `server/lib/audit/rendering-policy.ts`                              |    13 | TODO   |

### ga4 (14 files, 2,773 lines) -> `backend/internal/ga4`

| TS file (src/)                                                | Lines | Status |
| ------------------------------------------------------------- | ----: | ------ |
| `server/lib/ga4Client.ts`                                     |   474 | TODO   |
| `server/features/ga4/services/Ga4ReportingService.ts`         |   372 | TODO   |
| `server/features/ga4/services/Ga4ReportEnhancements.ts`       |   360 | TODO   |
| `server/features/ga4/services/Ga4ReportDefinitions.ts`        |   296 | TODO   |
| `server/features/ga4/services/SearchOpportunityService.ts`    |   293 | TODO   |
| `serverFunctions/ga4.ts`                                      |   220 | TODO   |
| `server/features/ga4/services/Ga4OrganicOverviewService.ts`   |   157 | TODO   |
| `server/features/ga4/services/Ga4Service.ts`                  |   146 | TODO   |
| `server/features/ga4/services/Ga4ReportNormalization.ts`      |   119 | TODO   |
| `server/features/ga4/services/Ga4MeasurementHealthService.ts` |    96 | TODO   |
| `server/features/ga4/services/ga4-test-fixtures.ts`           |    88 | TODO   |
| `server/features/ga4/repositories/Ga4ConnectionRepository.ts` |    66 | TODO   |
| `server/lib/ga4Errors.ts`                                     |    59 | TODO   |
| `server/features/ga4/services/Ga4Dates.ts`                    |    27 | TODO   |

### sam (9 files, 1,964 lines) -> `backend/internal/sam`

| TS file (src/)                                | Lines | Status |
| --------------------------------------------- | ----: | ------ |
| `server/features/sam/SamChatAgent.ts`         |   611 | TODO   |
| `server/features/sam/samChatTools.ts`         |   430 | TODO   |
| `server/features/sam/samTurnTelemetry.ts`     |   357 | TODO   |
| `server/features/sam/samToolOutput.ts`        |   179 | TODO   |
| `server/features/sam/SamSessionRepository.ts` |   110 | TODO   |
| `server/features/sam/samSkills.ts`            |    97 | TODO   |
| `serverFunctions/sam.ts`                      |    86 | TODO   |
| `server/features/sam/samSystemPrompt.ts`      |    59 | TODO   |
| `serverFunctions/samAccess.ts`                |    35 | TODO   |

### reports (9 files, 1,499 lines) -> `backend/internal/reports`

| TS file (src/)                                                     | Lines | Status |
| ------------------------------------------------------------------ | ----: | ------ |
| `server/features/reports/services/ReportService.ts`                |   326 | TODO   |
| `server/features/reports/sharePage.tsx`                            |   283 | TODO   |
| `server/features/reports/repositories/ReportRepository.ts`         |   276 | TODO   |
| `server/features/reports/services/ReportTemplateService.ts`        |   184 | TODO   |
| `serverFunctions/reports.ts`                                       |   136 | TODO   |
| `server/features/reports/repositories/ReportTemplateRepository.ts` |   113 | TODO   |
| `server/features/reports/reportSocialImage.tsx`                    |    89 | TODO   |
| `serverFunctions/reportTemplates.ts`                               |    69 | TODO   |
| `server/features/reports/shareAccess.ts`                           |    23 | TODO   |

### billing (10 files, 1,407 lines) -> `backend/internal/billing`

| TS file (src/)                               | Lines | Status |
| -------------------------------------------- | ----: | ------ |
| `server/billing/subscription.ts`             |   626 | TODO   |
| `serverFunctions/billing.ts`                 |   144 | TODO   |
| `server/billing/loops-sync.ts`               |   107 | TODO   |
| `server/billing/autumn-webhook.ts`           |    96 | TODO   |
| `server/billing/svix.ts`                     |    95 | TODO   |
| `server/billing/autumn.ts`                   |    86 | TODO   |
| `server/billing/customer-status-model.ts`    |    82 | TODO   |
| `server/billing/lifecycle-events.ts`         |    82 | TODO   |
| `server/billing/customer-status-sync.ts`     |    71 | TODO   |
| `server/billing/loops-contact-properties.ts` |    18 | TODO   |

### ai-search (6 files, 1,303 lines) -> `backend/internal/ai-search`

| TS file (src/)                                             | Lines | Status |
| ---------------------------------------------------------- | ----: | ------ |
| `server/features/ai-search/services/brandLookup.ts`        |   392 | TODO   |
| `server/features/ai-search/services/promptExplorer.ts`     |   354 | TODO   |
| `server/features/ai-search/services/brandLookupShaping.ts` |   254 | TODO   |
| `server/features/ai-search/services/shareOfVoice.ts`       |   132 | TODO   |
| `server/features/ai-search/services/citedSources.ts`       |   130 | TODO   |
| `serverFunctions/ai-search.ts`                             |    41 | TODO   |

### gsc (8 files, 1,234 lines) -> `backend/internal/gsc`

| TS file (src/)                                                | Lines | Status |
| ------------------------------------------------------------- | ----: | ------ |
| `server/features/gsc/services/GscService.ts`                  |   283 | TODO   |
| `serverFunctions/searchPerformance.ts`                        |   219 | TODO   |
| `server/lib/gscClient.ts`                                     |   175 | TODO   |
| `server/features/gsc/searchAnalytics.ts`                      |   171 | TODO   |
| `serverFunctions/gsc.ts`                                      |   152 | TODO   |
| `server/features/gsc/searchPerformanceReport.ts`              |   145 | TODO   |
| `server/features/gsc/repositories/GscConnectionRepository.ts` |    62 | TODO   |
| `server/lib/gscErrors.ts`                                     |    27 | TODO   |

### backlinks (7 files, 1,022 lines) -> `backend/internal/backlinks`

| TS file (src/)                                                     | Lines | Status |
| ------------------------------------------------------------------ | ----: | ------ |
| `server/features/backlinks/services/backlinksServiceData.ts`       |   385 | TODO   |
| `server/features/backlinks/services/backlinksApiFilters.ts`        |   179 | TODO   |
| `server/features/backlinks/services/BacklinksService.ts`           |   154 | TODO   |
| `server/features/backlinks/services/backlinksOverviewSchema.ts`    |   105 | TODO   |
| `server/features/backlinks/services/backlinksRowMappers.ts`        |    73 | TODO   |
| `server/features/backlinks/services/backlinksSubfolderOverview.ts` |    65 | TODO   |
| `serverFunctions/backlinks.ts`                                     |    61 | TODO   |

### google (7 files, 681 lines) -> `backend/internal/google`

| TS file (src/)                                      | Lines | Status |
| --------------------------------------------------- | ----: | ------ |
| `server/features/google/googleOAuth.ts`             |   406 | TODO   |
| `server/features/google/googleOAuthState.ts`        |   124 | TODO   |
| `server/features/google/GoogleAccountRepository.ts` |    60 | TODO   |
| `server/features/google/oauth-config.ts`            |    27 | TODO   |
| `serverFunctions/googleAccounts.ts`                 |    23 | TODO   |
| `server/features/google/GoogleAccountService.ts`    |    21 | TODO   |
| `server/features/google/googleIdToken.ts`           |    20 | TODO   |

### dashboard (3 files, 381 lines) -> `backend/internal/dashboard`

| TS file (src/)                                                         | Lines | Status |
| ---------------------------------------------------------------------- | ----: | ------ |
| `server/features/dashboard/services/DashboardService.ts`               |   268 | TODO   |
| `serverFunctions/dashboard.ts`                                         |    79 | TODO   |
| `server/features/dashboard/repositories/BacklinkSnapshotRepository.ts` |    34 | TODO   |

### referrals (2 files, 355 lines) -> `backend/internal/referrals`

| TS file (src/)                 | Lines | Status |
| ------------------------------ | ----: | ------ |
| `server/referrals/dub.ts`      |   289 | TODO   |
| `server/referrals/dub-sale.ts` |    66 | TODO   |

### email (2 files, 316 lines) -> `backend/internal/email`

| TS file (src/)                 | Lines | Status |
| ------------------------------ | ----: | ------ |
| `server/email/loops.ts`        |   179 | TODO   |
| `server/email/loops-client.ts` |   137 | TODO   |

### gdpr (1 files, 297 lines) -> `backend/internal/gdpr`

| TS file (src/)                   | Lines | Status |
| -------------------------------- | ----: | ------ |
| `server/gdpr/storage-erasure.ts` |   297 | TODO   |

### activation (2 files, 226 lines) -> `backend/internal/activation`

| TS file (src/)                                                    | Lines | Status |
| ----------------------------------------------------------------- | ----: | ------ |
| `server/features/activation/repositories/ActivationRepository.ts` |   184 | TODO   |
| `server/features/activation/mcpActivation.ts`                     |    42 | TODO   |

### branding (4 files, 187 lines) -> `backend/internal/branding`

| TS file (src/)                                                | Lines | Status |
| ------------------------------------------------------------- | ----: | ------ |
| `server/features/branding/brandBar.tsx`                       |    77 | TODO   |
| `server/features/branding/repositories/BrandingRepository.ts` |    50 | TODO   |
| `serverFunctions/branding.ts`                                 |    32 | TODO   |
| `server/features/branding/services/BrandingService.ts`        |    28 | TODO   |

### ahrefs (1 files, 119 lines) -> `backend/internal/backlinks`

| TS file (src/)              | Lines | Status |
| --------------------------- | ----: | ------ |
| `serverFunctions/ahrefs.ts` |   119 | TODO   |
