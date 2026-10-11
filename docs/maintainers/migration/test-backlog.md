# Deferred migration test backlog

New test files are intentionally deferred until the final migration phase.
Update this ledger in each migration PR so the remaining test work and its file
count stay visible. Existing tests remain in place and continue to be useful
while features move to Go.

## Pending new test files

Current confirmed count: **6 files**.

| Feature / slice                  | Test file to add at the end                                           | Coverage to add                                                                                                                                    |
| -------------------------------- | --------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| GA4 dashboard Go API cutover     | `src/client/features/dashboard/ga4DashboardApi.test.ts`               | Maps Go overview totals and trend rows, fills missing dates with zero sessions, and handles disconnected / expired / inaccessible GA4 connections. |
| Projects API cutover             | `src/client/features/projects/projectApi.test.ts`                     | Covers project list/create/update/archive/restore/access request mapping and response parsing.                                                     |
| Project context API cutover      | `src/client/features/projects/projectContextApi.test.ts`              | Covers context read/update paths, payloads, and response parsing.                                                                                  |
| GSC connection API cutover       | `src/client/features/integrations/gscApi.test.ts`                     | Covers grant status, connection status, site listing/selection, and disconnect request mapping.                                                    |
| GSC performance API cutover      | `src/client/features/search-performance/searchPerformanceApi.test.ts` | Covers report/table/export payload normalization and connected/disconnected response parsing.                                                      |
| Dashboard activation API cutover | `src/client/features/dashboard/dashboardApi.test.ts`                  | Covers activation/overview parsing and click, dismiss, and GA4-card dismissal requests.                                                            |

This is a running count, not the final migration total. Add every newly
identified test file here as its feature is migrated, then write the listed
tests in the final test phase and clear the pending count.

The existing `src/client/features/dashboard/DashboardOnboarding.test.ts` also
needs its server-function mock updated to mock `dashboardApi` in the final test
phase; it does not add a new test file to the count.

## Existing test inventory at migration start

- TypeScript server and server-function test files: **127**.
- Go test files (`*_test.go`): **114**.

These are baseline counts, not files to rewrite automatically. During the final
test phase, review existing TypeScript coverage against the Go implementations
and add or move tests only where needed to cover migrated behavior.
