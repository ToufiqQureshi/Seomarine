# Migration progress

Tracks verified TypeScript-to-Go migration work. A status only advances after its listed code and tests exist; PR links are added when opened.

Last updated: 2026-10-08

## Current phase

- Phase A, item A1: handler cleanup and router mounts.
- Status: IN PROGRESS; local branch `mig/foundation`, no PR opened.
- Next: finish A1 review/checks, then create a draft PR before starting A2.

## Phase A

| Item                                             | Status      | PR          | Notes                                                                                                                                                                                                                                                                                                                                                                                        |
| ------------------------------------------------ | ----------- | ----------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `.gitattributes`                                 | DONE        | Base branch | LF normalization already exists on `main`.                                                                                                                                                                                                                                                                                                                                                   |
| A1 `httpx` response helpers and feature handlers | IN PROGRESS | Not opened  | Analytics and billing handlers are in their feature packages; mounts are called by the root router. Collect validation and client-IP tests were moved into `analytics`. Analytics, billing, and httpx package tests plus `go vet` pass. Full race/coverage tests cannot run on this Windows host because CGO is disabled and Application Control blocks Go test tools; Linux CI is required. |
| A2 `platform/pgdb`                               | TODO        | —           | Pool, transaction helper, and real-Postgres test helper not implemented.                                                                                                                                                                                                                                                                                                                     |
| A3 `platform/jobs`                               | TODO        | —           | Postgres queue, `SKIP LOCKED` claim, retries, timeout, idempotency, and worker runner not implemented.                                                                                                                                                                                                                                                                                       |
| A4 `platform/dataforseo`                         | TODO        | —           | Client, bounded retries, timeout, and per-organization cost tracking not implemented.                                                                                                                                                                                                                                                                                                        |
| A5 `platform/entitlements`                       | TODO        | —           | Allow-all `Check` stub not implemented.                                                                                                                                                                                                                                                                                                                                                      |
| A6 OpenAPI/code generation                       | TODO        | —           | Per-feature specs, generation command, and stale-code CI check not implemented.                                                                                                                                                                                                                                                                                                              |
| A7 migration guide/template                      | TODO        | —           | Per-feature progress template and ownership guide not implemented.                                                                                                                                                                                                                                                                                                                           |
| A8 remaining foundation work                     | TODO        | —           | Depends on review of A1–A7.                                                                                                                                                                                                                                                                                                                                                                  |

## Migration modules

| Module                                     | Status      | PR         | Known gaps                                                                                               |
| ------------------------------------------ | ----------- | ---------- | -------------------------------------------------------------------------------------------------------- |
| analytics                                  | IN PROGRESS | Not opened | Go routes exist; remaining TS server functions and frontend calls are not migrated.                      |
| billing                                    | IN PROGRESS | Not opened | Go status/checkout/webhook routes exist; usage, limits, cancel, frontend switch, and TS deletion remain. |
| organizations/workspaces                   | TODO        | —          | Copilot-owned work is in another worktree; not included in this foundation branch.                       |
| onboarding/config                          | TODO        | —          | Not ported.                                                                                              |
| projects/project-context                   | TODO        | —          | Not ported.                                                                                              |
| domain/serp-locations/keywords             | TODO        | —          | Not ported; assigned lane work is separate.                                                              |
| rank-tracking                              | TODO        | —          | Not ported.                                                                                              |
| audit/crawler/lighthouse                   | TODO        | —          | Not ported.                                                                                              |
| backlinks/AI search/SAM                    | TODO        | —          | Not ported.                                                                                              |
| reports/templates/branding                 | TODO        | —          | Not ported.                                                                                              |
| GA4/GSC/Google accounts/search performance | TODO        | —          | Not ported.                                                                                              |
| dashboard/activation/referrals/GDPR/email  | TODO        | —          | Not ported.                                                                                              |
| MCP and legacy backend cleanup             | TODO        | —          | Must remain last.                                                                                        |

## Legacy modules

| Module                                          | Status      | PR  | Known gaps                                                               | TS lines -> Go lines |
| ----------------------------------------------- | ----------- | --- | ------------------------------------------------------------------------ | -------------------- |
| analytics                                       | IN PROGRESS | TBD | Need strict tenant checks, feature-folder extraction, parity validation. | TBD                  |
| billing                                         | IN PROGRESS | TBD | Need checkout/webhook parity and subscription state checks.              | TBD                  |
| organizations/workspaces                        | TODO        | TBD | Missing shared Go ownership and session validation audit.                | TBD                  |
| onboarding/config                               | TODO        | TBD | Needs auth/session flow mapping to Go.                                   | TBD                  |
| projects/project-context                        | TODO        | TBD | Project membership and access checks need crisp repo-level tests.        | TBD                  |
| domain                                          | TODO        | TBD | Needs parity fixtures for domain retrieval and normalization.            | TBD                  |
| serp-locations                                  | TODO        | TBD | India city-level mapping and location parity still pending.              | TBD                  |
| keywords                                        | TODO        | TBD | Research + saved keyword flows need feature split.                       | TBD                  |
| rank-tracking                                   | TODO        | TBD | Scheduled job and polling parity not yet ported.                         | TBD                  |
| audit/crawler/lighthouse                        | TODO        | TBD | Needs crawler access and job queue parity.                               | TBD                  |
| backlinks                                       | TODO        | TBD | Need Ahrrefs/adapter parity coverage.                                    | TBD                  |
| reports/reportTemplates/branding                | TODO        | TBD | Layout and share/PDF parity to confirm.                                  | TBD                  |
| ga4/gsc/googleAccounts                          | TODO        | TBD | OAuth and token refresh parity still pending.                            | TBD                  |
| searchPerformance                               | TODO        | TBD | Contract mapping and analytics parity not yet checked.                   | TBD                  |
| ai-search/sam/samAccess                         | TODO        | TBD | Search assistant flows not yet ported.                                   | TBD                  |
| dashboard/activation/referrals/gdpr/email       | TODO        | TBD | Needs workspace data flow review.                                        | TBD                  |
| MCP server                                      | TODO        | TBD | Must be last major feature migration.                                    | TBD                  |
| src/server, src/serverFunctions, workers config | TODO        | TBD | Delete only after each feature is proven and UI switched to Go API.      | TBD                  |

## Blockers

- No product/code blocker recorded. The Windows Application Control policy can block Go test executables; report affected commands and rely on Linux CI for those checks.
