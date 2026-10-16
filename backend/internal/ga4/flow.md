# Google Analytics reports: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{projectId}/ga4/{reports/run|overview/organic|measurement-health|search-opportunities}
- **Source:** `handler.go`, `service.go`, `overview.go`, `measurement_health.go`, `search_opportunity.go`

## Request or job flow

Handler authorizes project and selected property; Google API client refreshes the token once if needed; service validates report dimensions/date range, fetches GA4/GSC data and returns normalized rows/health findings.

## Dependencies and cost boundary

Google Analytics Admin/Data APIs and GSC; encrypted Google tokens; Postgres selected property.

## Failure, security and data invariants

- Google calls are quota-limited; use bounded reads/retries and avoid duplicate queries.
- Never expose OAuth tokens; property must belong to the connected organization/project.
- GA4 and GSC metrics differ in definitions/attribution; do not silently equate them.
