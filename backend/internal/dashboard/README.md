# Dashboard response contracts

Defines shared dashboard response types; request behavior is implemented by dashboardoverview and activation packages.

This is an internal developer/AI-agent guide, not customer-facing product copy. Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for cost, data and security invariants.

## Source map

- `types.go`

## API/trigger

No standalone route; consumed by `dashboardoverview` handlers and the dashboard client.
