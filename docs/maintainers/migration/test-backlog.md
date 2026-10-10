# Deferred migration test backlog

New test files are intentionally deferred until the final migration phase.
Update this ledger in each migration PR so the remaining test work and its file
count stay visible. Existing tests remain in place and continue to be useful
while features move to Go.

## Pending new test files

Current confirmed count: **53 files**.

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
| Crawler access API cutover       | `src/client/features/crawler-access/crawlerAccessApi.test.ts`          | Covers credential list/save/delete, Shopify signature problems, project authorization, and secret-free responses.                                  |
| Ahrefs Go service                | `backend/internal/ahrefs/service_test.go`                             | Covers normalization, cache hits including null ratings, batching, provider failures, and result key preservation.                                 |
| Ahrefs Go handler                | `backend/internal/ahrefs/handler_test.go`                             | Covers session/project authorization, request validation, and responses.                                                                           |
| Autumn usage Go handler          | `backend/internal/billing/usage_test.go`                              | Covers hosted/self-hosted behavior, pagination, 429 retries, range validation, and provider errors.                                                |
| Autumn credit-balance Go client | `backend/internal/billing/autumn_test.go`                             | Covers missing configuration, request headers/payload, null balances, provider failures, timeouts, and response bounds.                                |
| MCP whoami credit lookup         | `backend/internal/mcp/whoami_test.go`                                  | Covers hosted/self-hosted mode, zero and unknown balances, scopes, and upstream failures without charging credits.                                     |
| Organization Go API              | `backend/internal/auth/organization_handler_test.go`                  | Covers membership context, active organization switching, team listing permissions, and ownership transfer.                                        |
| Go MCP domain keyword suggestions tool | `backend/internal/mcp/domain_keyword_suggestions_test.go`         | Covers project auth, project-market fallback, location/language validation, target scopes, ranked keyword mapping, and empty results. |
| Go MCP ranked keywords tool | `backend/internal/mcp/ranked_keywords_test.go` | Covers project auth, scope aliases, market selector, filter budget, result types, pagination, sorting, row mapping, and provider errors. |
| Go MCP SERP results tool | `backend/internal/mcp/serp_results_test.go` | Covers project auth, query count/depth bounds, per-query market/location resolution, mixed success/failure batches, row trimming, and empty results. |
| Go MCP create rank tracker tool | `backend/internal/mcp/rank_tracker_create_test.go` | Covers project auth, domain defaults, market/language validation, city registry checks, schedule/time-zone bounds, duplicate configs, and manual no-spend defaults. |
| Go MCP estimate rank tracker cost tool | `backend/internal/mcp/rank_tracker_cost_test.go` | Covers project auth, tracker scoping, count/keyword precedence, bounded additions, live and scheduled cost shapes, and no-check behavior. |
| Go MCP run rank tracker tool | `backend/internal/mcp/rank_tracker_run_test.go` | Covers authorization, positive cost approval, live estimate recheck, paid-plan gates, queued job start, active-run response, and failures. |
| Go MCP keyword metrics tool | `backend/internal/mcp/keyword_metrics_test.go` | Covers project auth, 1-700 keyword bounds, market and language resolution, Labs versus Ads provider selection, clickstream trends, nullable fields, sorting, and provider failures. |
| Go MCP keyword research tool | `backend/internal/mcp/research_keywords_test.go` | Covers authorization, 1-5 independent seeds, market/language selection, local locations, limit defaults, clickstream/group options, and per-seed failures. |
| Go MCP URL inspection tool | `backend/internal/mcp/inspect_urls_test.go` | Covers authorization, URL validation, property resolution, language handling, per-URL success/failure, unconnected grants, and metadata. |
| Go MCP SERP competitors tool | `backend/internal/mcp/serp_competitors_test.go` | Covers project authorization, keywords/market/result-type validation, Labs-only language selection, domain exclusions, sorting, defaults, and provider failures. |
| Go MCP local SERP results tool | `backend/internal/mcp/local_serp_results_test.go` | Covers project authorization, coordinate/zoom validation, market language defaults, device/search type/depth defaults, trimmed output fields, and provider errors. |
| Go MCP business questions tool | `backend/internal/mcp/business_questions_test.go` | Covers project authorization, exactly-one business identifier, coordinate/radius/depth validation, language defaults, empty results, answer trimming, and provider errors. |
| Go MCP business categories tool | `backend/internal/mcp/business_categories_test.go` | Covers project authorization, query/limit bounds, cached categories, case-insensitive filtering, free unmetered retrieval, and provider errors. |
| Go MCP local business search tool | `backend/internal/mcp/local_business_search_test.go` | Covers authorization, coordinate/radius and category bounds, rating/review filters, claimed state, sorting/paging, trimmed row fields, and empty/provider failures. |
| Go MCP business profile tool | `backend/internal/mcp/business_profile_test.go` | Covers authorization, exactly-one identifier, location/code defaults, coordinate/radius validation, check URL merge, empty profile, and provider failures. |
| Go MCP business reviews tool | `backend/internal/mcp/business_reviews_test.go` | Covers auth, identifier/task ID validation, billed task creation, Google/extended endpoints, resumable polling, empty results, review projection/totals, and collection failures. |
| Go MCP business updates tool | `backend/internal/mcp/business_updates_test.go` | Covers authorization, one identifier/task ID, location defaults, billed task posting, free resumable polling, empty results, output projection, and failures. |
| Go MCP local rank grid tool | `backend/internal/mcp/local_rank_grid_test.go` | Covers authorization, grid geometry and zoom, provider concurrency and billing, identity matching, partial/all failures, rank summaries, and rendered output. |
| Go MCP domain overview tool       | `backend/internal/mcp/domain_overview_test.go`                      | Covers project auth, project-market fallback, language/location validation, scope aliases, organic metrics, backlink summary, and provider errors. |
| Go MCP backlinks profile tool     | `backend/internal/mcp/backlinks_profile_test.go`                    | Covers project auth, target scope, defaults, pagination, sort mapping, filters, spam threshold, grouping mode, row summaries, and provider errors. |
| Go MCP backlinks overview tool   | `backend/internal/mcp/backlinks_overview_test.go`                   | Covers project auth, scope normalization, summary/trend payloads, subfolder behavior, spam filtering, top referring domains, and provider errors. |
| Go MCP audit issue read tool     | `backend/internal/mcp/audit_issues_test.go`                         | Covers project authorization, latest/requested audit resolution, severity/type filters, deterministic sorting, summaries, row limits, and issue remediation metadata. |
| Go MCP site-audit start tool     | `backend/internal/mcp/run_site_audit_test.go`                       | Covers project membership, tier/capacity gates, URL/SSRF validation, optional Lighthouse/rendering flags, and the queued audit response. |
| Hosted Go sign-out endpoint      | `backend/internal/auth/session_handler_test.go`                       | Covers hosted route ownership, valid/revoked/invalid cookies, cookie expiry, and database failure responses.                                          |
| Hosted Go get-session endpoint   | `backend/internal/auth/current_session_test.go`                      | Covers the Better Auth JSON shape and analyticsOptedOut field, secure/local cookie refresh after one day, invalid, expired and revoked sessions, null optional fields, and DB failures. |
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
