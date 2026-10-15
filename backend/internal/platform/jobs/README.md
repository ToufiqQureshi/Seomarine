# Postgres background job queue

Enqueues, leases, retries and completes asynchronous jobs with optional idempotency keys.

Internal package guide: [flow.md](flow.md) describes the code path; [decision.md](decision.md) records invariants to preserve.

## Source map

- `queue.go`
- `worker.go`
- `queue_test.go`
- `queue_integration_test.go`
