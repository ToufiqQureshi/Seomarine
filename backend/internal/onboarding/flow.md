# Onboarding state: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/onboarding/answers/{get|save}; POST gsc-nudge/dismiss
- **Source:** `handler.go`, `onboarding.go`, `repository.go`

## Request or job flow

Session-scoped UI reads/saves bounded answer state; repository persists it; separate nudge dismissal does not implicitly claim OAuth success.

## Dependencies and cost boundary

Postgres onboarding records; auth session; Google integration owns OAuth.

## Failure, security and data invariants

- Scope answers to the current user and keep dismissal independent from actual integration status.
- Repeated save/dismiss requests are idempotent.
- Onboarding state must never grant project access or bypass plan checks.
