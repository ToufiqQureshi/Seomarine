# Google Search Console: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{projectId}/gsc/...; `/gsc/search-performance/...`; URL inspection routes
- **Source:** `connection_handler.go`, `performance.go`, `inspection.go`, `client.go`

## Request or job flow

Project request resolves saved property and grant; client validates filter grammar and API budget; Google API returns performance/inspection results; handlers normalize rows while retaining per-URL outcomes.

## Dependencies and cost boundary

Google Search Console API; encrypted Google grants; selected property in Postgres.

## Failure, security and data invariants

- Property and grant must belong to the authorized organization/project.
- Google quotas require bounded read-only retries; do not retry writes blindly.
- Treat provider row limits and inspection errors accurately; null/empty is not proof of no search demand.
