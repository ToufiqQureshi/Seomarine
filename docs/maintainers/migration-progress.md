# Migration progress

Tracks verified TypeScript-to-Go migration work. A status advances only after the code and tests exist. Draft PRs and known gaps are listed here.

Last updated: 2026-10-08

## Current phase

- Phase A, item A2: Postgres pool and transaction helpers.
- Status: IN PROGRESS on `mig/foundation-pgdb`, stacked on [A1 PR #8](https://github.com/ToufiqQureshi/Seomarine/pull/8).
- Next: finish pgdb checks and open its draft PR against `mig/foundation`.

## Phase A

| Item | Status | PR | Notes |
| --- | --- | --- | --- |
| `.gitattributes` | DONE | Base branch | LF normalization already exists on `main`. |
| A1 httpx helpers and feature handlers | IN PROGRESS | [PR #8](https://github.com/ToufiqQureshi/Seomarine/pull/8) | Analytics, billing, and httpx package tests plus `go vet` pass. CI is running. Full race/coverage tests cannot run locally because CGO is disabled and Windows Application Control blocks Go test tools. |
| A2 `platform/pgdb` | IN PROGRESS | Not opened | Pool, transaction helper, and real-Postgres integration tests are being added on a branch stacked on A1. |
| A3 `platform/jobs` | TODO | - | Postgres queue, `SKIP LOCKED` claim, retries, timeout, idempotency, and worker runner not implemented. |
| A4 `platform/dataforseo` | TODO | - | Client, bounded retries, timeout, and per-organization cost tracking not implemented. |
| A5 `platform/entitlements` | TODO | - | Allow-all `Check` stub not implemented. |
| A6 OpenAPI and code generation | TODO | - | Per-feature specs, generation command, and stale-code CI check not implemented. |
| A7 migration guide and template | TODO | - | Per-feature progress template and ownership guide not implemented. |
| A8 remaining foundation work | TODO | - | Depends on review of A1-A7. |

## Migration modules

| Module | Status | PR | Known gaps |
| --- | --- | --- | --- |
| analytics | IN PROGRESS | [PR #8](https://github.com/ToufiqQureshi/Seomarine/pull/8) | Go routes exist; remaining TS server functions and frontend calls are not migrated. |
| billing | IN PROGRESS | [PR #8](https://github.com/ToufiqQureshi/Seomarine/pull/8) | Go status, checkout, and webhook routes exist; usage, limits, cancel, frontend switch, and TS deletion remain. |
| organizations/workspaces | TODO | - | Copilot-owned work is in another worktree and is not included in this branch. |
| onboarding/config | TODO | - | Not ported. |
| projects/project-context | TODO | - | Not ported. |
| domain/serp-locations/keywords | TODO | - | Not ported; lane work is separate. |
| rank-tracking | TODO | - | Not ported. |
| audit/crawler/lighthouse | TODO | - | Not ported. |
| backlinks/AI search/SAM | TODO | - | Not ported. |
| reports/templates/branding | TODO | - | Not ported. |
| GA4/GSC/Google accounts/search performance | TODO | - | Not ported. |
| dashboard/activation/referrals/GDPR/email | TODO | - | Not ported. |
| MCP and legacy backend cleanup | TODO | - | Must remain last. |
