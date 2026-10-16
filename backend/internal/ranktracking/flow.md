# Rank tracking engine: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{projectId}/rank-tracking/...; background rank job handlers
- **Source:** `handler.go`, `checks.go`, `queued.go`, `scheduler.go`

## Request or job flow

User saves a tracker or starts a run; service estimates/checks ceiling, reserves credits and enqueues jobs; worker obtains rank tasks, persists idempotently, resumes partial runs and settles actual spend; UI polls state.

## Dependencies and cost boundary

DataForSEO SERP; Postgres configs/runs/tasks/snapshots; Redis jobs/locks; billing/spend guards.

## Failure, security and data invariants

- Check estimated ceiling again before execution; a browser estimate is never authority.
- Use idempotency and task state to prevent duplicate paid work across retries/restarts.
- Scope caps to the workspace causing spend; expose partial failures and enforce schedule/pause rules.
