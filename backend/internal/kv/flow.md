# Key-value and Redis access: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** No public HTTP route; called by feature services and middleware.
- **Source:** `kv.go`

## Request or job flow

Startup config creates the Redis client; callers use context-aware bounded commands; service behavior decides whether a cache/rate-limit outage fails open or closed.

## Dependencies and cost boundary

Redis.

## Failure, security and data invariants

- Every Redis call must use request context and bounded timeouts.
- Do not treat cache data as authoritative customer records.
- Choose fail-open/fail-closed per security and cost risk; document it at the consumer.
