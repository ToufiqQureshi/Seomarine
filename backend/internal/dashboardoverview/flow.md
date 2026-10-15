# Dashboard overview: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{projectId}/dashboard/overview; POST .../dashboard/backlinks/refresh
- **Source:** `handler.go`, `service.go`, `repository.go`

## Request or job flow

Dashboard read gathers project domain, latest audit and stored backlink snapshot from Postgres; an explicit refresh mutation checks access/billing/provider availability and updates the snapshot.

## Dependencies and cost boundary

Postgres snapshots; optional DataForSEO backlinks provider; plans and billing.

## Failure, security and data invariants

- Opening a dashboard must remain read-only and must not trigger paid provider requests.
- Refresh is an explicit action with server-side plan/credit checks and a visible stale/failure state.
- All database reads and refresh writes are project-scoped.
