# Onboarding state

Stores first-run answers and Search Console nudge state for the signed-in user.

This is an internal developer/AI-agent guide, not customer-facing product copy. Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for cost, data and security invariants.

## Source map

- `handler.go`
- `onboarding.go`
- `repository.go`

## API/trigger

POST /api/v1/onboarding/answers/{get|save}; POST gsc-nudge/dismiss
