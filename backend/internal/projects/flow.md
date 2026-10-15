# Project lifecycle: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{list|create|archived|restore}; POST /{projectId}/{get|update|archive}
- **Source:** `handler.go`, `projects.go`

## Request or job flow

Session request lists workspace projects; mutations enforce organization membership/plan/site limits; repository persists project and market; feature middleware resolves the authorized project for downstream APIs.

## Dependencies and cost boundary

Postgres projects and organization membership; plan catalog; projectcontext/dashboard consumers.

## Failure, security and data invariants

- Treat every project ID as untrusted and scope by the caller’s organization.
- Create/restore must enforce current plan limits under concurrency where required.
- Archiving must not erase customer data; restore must reconnect feature access without changing ownership.
