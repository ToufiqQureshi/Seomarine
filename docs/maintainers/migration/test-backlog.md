# Deferred migration test backlog

New test files are intentionally deferred until the final migration phase.
Update this ledger in each migration PR so the remaining test work and its file
count stay visible. Existing tests remain in place and continue to be useful
while features move to Go.

## Pending new test files

Current confirmed count: **26 files**.

| Feature / slice                  | Test file to add at the end                                           | Coverage to add                                                                                                                                    |
| -------------------------------- | --------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| GA4 dashboard Go API cutover     | `src/client/features/dashboard/ga4DashboardApi.test.ts`               | Maps Go overview totals and trend rows, fills missing dates with zero sessions, and handles disconnected / expired / inaccessible GA4 connections. |
| Projects API cutover             | `src/client/features/projects/projectApi.test.ts`                     | Covers project list/create/update/archive/restore/access request mapping and response parsing.                                                     |
| Project context API cutover      | `src/client/features/projects/projectContextApi.test.ts`              | Covers context read/update paths, payloads, and response parsing.                                                                                  |
| GSC connection API cutover       | `src/client/features/integrations/gscApi.test.ts`                     | Covers grant status, connection status, site listing/selection, and disconnect request mapping.                                                    |
| GSC performance API cutover      | `src/client/features/search-performance/searchPerformanceApi.test.ts` | Covers report/table/export payload normalization and connected/disconnected response parsing.                                                      |
| Dashboard activation API cutover | `src/client/features/dashboard/dashboardApi.test.ts`                  | Covers activation/overview parsing, click and dismiss requests, GA4-card dismissal, and daily backlink snapshot refresh.                           |
| SAM session API cutover          | `src/client/features/sam/samApi.test.ts`                              | Covers session listing, creation, archive payloads, and response parsing.                                                                          |
| Rank tracking API cutover        | `src/client/features/rank-tracking/rankTrackingApi.test.ts`           | Covers Go route payloads, config summaries, ranking results/history, and mutation response mapping.                                                |
| Reports API cutover              | `src/client/features/reports/reportApi.test.ts`                       | Covers report metadata, template names, sharing state, template list/save/delete, and errors.                                                      |
| Onboarding API cutover           | `src/client/features/onboarding/onboardingApi.test.ts`                | Covers saved-answer reads/writes, completion, and Search Console nudge dismissal.                                                                  |
| Runtime setup API cutover        | `src/client/lib/appConfigApi.test.ts`                                 | Covers SEO provider-key status and SAM access setup status from Go.                                                                                |
| Ahrefs domain-rating API cutover | `src/client/features/backlinks/ahrefsApi.test.ts`                     | Covers project-scoped Go requests, original-domain result keys, unknown ratings, and API errors.                                                   |
| Billing usage API cutover        | `src/client/features/billing/billingApi.test.ts`                      | Covers hosted usage-event date ranges, event properties, self-hosted empty results, and upstream errors.                                           |
| Organization context API cutover | `src/client/features/team/organizationApi.test.ts`                    | Covers active organization context, membership listing, switching requests, and response parsing.                                                  |
| Workspace merge API cutover      | `src/client/features/dashboard/workspaceApi.test.ts`                  | Covers status hiding outside Cloudflare Access mode, legacy counts, merge results, and errors.                                                     |
| Team API cutover                 | `src/client/features/team/teamApi.test.ts`                            | Covers team member/invitation visibility, ownership transfer, invite rate limits, and email-provider errors.                                       |
| Crawler access API cutover       | `src/client/features/crawler-access/crawlerAccessApi.test.ts`         | Covers credential list/save/delete, Shopify signature problems, project authorization, and secret-free responses.                                  |
| Ahrefs Go service                | `backend/internal/ahrefs/service_test.go`                             | Covers normalization, cache hits including null ratings, batching, provider failures, and result key preservation.                                 |
| Ahrefs Go handler                | `backend/internal/ahrefs/handler_test.go`                             | Covers session/project authorization, request validation, and responses.                                                                           |
| Autumn usage Go handler          | `backend/internal/billing/usage_test.go`                              | Covers hosted/self-hosted behavior, pagination, 429 retries, range validation, and provider errors.                                                |
| Organization Go API              | `backend/internal/auth/organization_handler_test.go`                  | Covers membership context, active organization switching, team listing permissions, and ownership transfer.                                        |
| Workspace merge Go service       | `backend/internal/workspace/merge_test.go`                            | Covers disabled auth modes, idempotency, project renaming, record repointing, and earliest activation milestones.                                  |
| Team invitation Go service       | `backend/internal/team/invitations_test.go`                           | Covers permissions, deduped pending invitations, daily limits, email payloads, and failed delivery.                                                |
| Crawler credential Go service    | `backend/internal/crawleraccess/service_test.go`                      | Covers Better Auth encryption compatibility, Shopify key verification, organization scoping, and audit resolution.                                 |
| Crawler credential Go handlers   | `backend/internal/crawleraccess/handler_test.go`                      | Covers permissions, secret-free list/save results, validation problems, and delete scoping.                                                        |
| Go self-host health endpoint     | `backend/internal/setupstatus/handler_test.go`                         | Covers hosted redaction, setup check statuses, base64 DataForSEO validation, database failure, and safe response details.                       |

This is a running count, not the final migration total. Add every newly
identified test file here as its feature is migrated, then write the listed
tests in the final test phase and clear the pending count.

The existing `src/client/features/dashboard/DashboardOnboarding.test.ts` needs
its server-function mock updated to mock `dashboardApi`, and
`src/client/features/dashboard/dashboardSteps.test.ts` needs its type import
moved to `dashboardApi` in the final test phase. These do not add new test files
to the count.

The existing `src/client/features/billing/BillingFeatureBreakdown.test.ts`
needs its server-function mock moved to `billingApi` in the final test phase.

The existing `src/client/features/onboarding/onboardingModel.test.ts` and
`src/client/features/integrations/googleSetupState.test.ts` still mock retired
server-function modules; update them to mock the onboarding and GSC client APIs.

The existing Go `backend/internal/audit/runner_test.go` and
`backend/internal/config/config_test.go` need cases for crawler credential
replay and hosted invitation URL configuration in the final test phase.

The existing `src/serverFunctions/searchPerformance.test.ts` and
`src/server/features/audit/services/CrawlerCredentialService.test.ts` cover
TypeScript implementations now served through Go. Move their behavior checks
into the listed Go tests and client API tests before removing those old test
files.

## Existing test inventory at migration start

- TypeScript server and server-function test files: **127**.
- Go test files (`*_test.go`): **114**.

These are baseline counts, not files to rewrite automatically. During the final
test phase, review existing TypeScript coverage against the Go implementations
and add or move tests only where needed to cover migrated behavior.
