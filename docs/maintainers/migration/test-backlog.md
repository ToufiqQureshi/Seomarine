# Deferred migration test backlog

New test files are intentionally deferred until the final migration phase.
Update this ledger in each migration PR so the remaining test work and its file
count stay visible. Existing tests remain in place and continue to be useful
while features move to Go.

## Pending new test files

Current confirmed count: **1 file**.

| Feature / slice              | Test file to add at the end                             | Coverage to add                                                                                                                                    |
| ---------------------------- | ------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| GA4 dashboard Go API cutover | `src/client/features/dashboard/ga4DashboardApi.test.ts` | Maps Go overview totals and trend rows, fills missing dates with zero sessions, and handles disconnected / expired / inaccessible GA4 connections. |

This is a running count, not the final migration total. Add every newly
identified test file here as its feature is migrated, then write the listed
tests in the final test phase and clear the pending count.

## Existing test inventory at migration start

- TypeScript server and server-function test files: **127**.
- Go test files (`*_test.go`): **114**.

These are baseline counts, not files to rewrite automatically. During the final
test phase, review existing TypeScript coverage against the Go implementations
and add or move tests only where needed to cover migrated behavior.
