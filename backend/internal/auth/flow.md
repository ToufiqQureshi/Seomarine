# Sessions and organization access: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** GET /api/auth/get-session; POST /api/auth/sign-out; organization and invitation endpoints
- **Source:** `auth.go`, `session.go`, `ba_organization.go`, `repository.go`

## Request or job flow

Request middleware verifies the signed cookie and database session; project middleware resolves organization membership; handlers expose Better Auth-compatible session/org operations.

## Dependencies and cost boundary

Better Auth tables and session cookie; project/team services.

## Failure, security and data invariants

- Cookie signatures, expiry and revocation are authoritative; never trust frontend session cache for API access.
- Project and active-organization IDs must be re-authorized from Postgres on every request.
- Organization removal/transfer must preserve owner invariants and clear stale active membership.
