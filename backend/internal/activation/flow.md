# Dashboard activation state: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{projectId}/activation/{get|click|dismiss|ga4-dismiss}
- **Source:** `handler.go`, `service.go`, `repository.go`

## Request or job flow

Dashboard requests state; handler authorizes project; service reads/writes activation rows; repository preserves first occurrence and caller-specific dismissals.

## Dependencies and cost boundary

Postgres activation state; dashboard UI.

## Failure, security and data invariants

- Scope every read by project organization and every dismissal by user.
- First-click timestamps are write-once; repeated clicks must not rewrite onboarding history.
- This package records interaction state only; it must not trigger provider refreshes.
