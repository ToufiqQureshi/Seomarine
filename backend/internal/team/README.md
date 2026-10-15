# Team invites and seats

Creates and manages organization invitations with role, seat and email-rate controls.

This is an internal developer/AI-agent guide, not customer-facing product copy. Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for cost, data and security invariants.

## Source map

- `handler.go`
- `invitations.go`

## API/trigger

POST /api/v1/organization/invitations; auth package handles accept/reject/remove/transfer operations.
