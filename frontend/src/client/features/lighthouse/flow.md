# Lighthouse issue details: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. User opens a completed audit and selects the Lighthouse issues view.
2. The screen requests the saved Lighthouse issue/export payload through the audit API.
3. Backend returns stored PageSpeed Insights/CrUX data; the UI renders scores, affected URLs and field-data status.
4. If data is unavailable, the UI shows the returned status without starting another Lighthouse run.

## Implementation map

- `issues/LighthouseIssuesScreen.tsx`
- `frontend/src/client/features/audit/auditApi.ts`
- `backend/internal/audit/lighthousereport.go`
- `backend/internal/audit/crux.go`

## API and service dependencies

Go package: `backend/internal/audit`; PSI and CrUX quotas are controlled there.

## Cost, security and failure boundaries

- Viewing stored details must not trigger another PageSpeed/CrUX request.
- Keep unavailable distinct from insufficient traffic data.
- Do not calculate or rewrite Google metrics from presentation code.
