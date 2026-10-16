# Account erasure: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** CLI workflow: `backend/cmd/erase-user`; not a public feature route.
- **Source:** `erase.go`

## Request or job flow

Operator runs dry-run plan by default; package reports candidate rows; explicit execute revalidates ownership and deletes user-owned data transactionally; shared-organization data is refused.

## Dependencies and cost boundary

Postgres tables; CLI and database connection.

## Failure, security and data invariants

- Dry-run remains default; destructive execution needs explicit operator intent.
- Never erase shared organization data when another member owns it.
- Report completion only after the whole transaction commits; preserve auditability without retaining erased personal data.
