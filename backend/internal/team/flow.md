# Team invites and seats: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/organization/invitations; auth package handles accept/reject/remove/transfer operations.
- **Source:** `handler.go`, `invitations.go`

## Request or job flow

Owner/admin sends invite; package checks permission, plan seats and Redis email budget atomically before mail; accepted invitation is consumed by auth organization flow.

## Dependencies and cost boundary

Postgres organization/invitations; Redis invite rate limits; Loops transactional email; plan catalog.

## Failure, security and data invariants

- Check role, membership and seat cap before sending mail or creating membership.
- Rate-limit by recipient and organization; fail closed if anti-abuse budget is unavailable.
- Never allow invitation flows to remove/replace owner without explicit ownership transfer.
