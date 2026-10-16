# Key-value and Redis access

Provides the shared Redis client primitives used for caches, rate limits, queues and short-lived state.

This is an internal developer/AI-agent guide, not customer-facing product copy. Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for cost, data and security invariants.

## Source map

- `kv.go`

## API/trigger

No public HTTP route; called by feature services and middleware.
