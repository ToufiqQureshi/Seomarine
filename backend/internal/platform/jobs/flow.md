# Postgres background job queue: flow

> Internal guide for developers and coding agents.

## Current behavior

Feature validates its input, enqueues a named job; worker claims an expiring lease using `SKIP LOCKED`, invokes the handler with a timeout/heartbeat, then completes or schedules bounded retry.

## Dependencies

Postgres `go_jobs`; worker lifecycle in server startup.

## Relevant code

- `queue.go`
- `worker.go`
- `queue_test.go`
- `queue_integration_test.go`
