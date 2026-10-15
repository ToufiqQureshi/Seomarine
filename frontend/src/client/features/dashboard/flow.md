# Project dashboard: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. Opening a project dashboard starts project-scoped queries for summary and activation state.
2. Cards render available snapshots and setup actions; dismissal/click actions persist activation state.
3. A user-triggered backlink refresh can call the backend provider; ordinary dashboard reads use stored snapshots.
4. GA4 card data comes from the linked property when one is selected.

## Implementation map

- `DashboardPage.tsx`
- `dashboardApi.ts`
- `ga4DashboardApi.ts`
- `backend/internal/dashboardoverview`
- `backend/internal/activation`
- `backend/internal/ga4`

## API and service dependencies

Go packages: `dashboardoverview`, `activation` and `ga4`; optional backlink refresh uses `backlinks`.

## Cost, security and failure boundaries

- Dashboard loads must not trigger expensive provider refreshes implicitly; refresh stays an explicit action.
- Refresh cost and plan checks stay server-side; stale snapshots should be labeled, not silently refreshed.
- Activation mutations are user/project-scoped and first-use timestamps remain stable.
