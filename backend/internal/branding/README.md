# Report branding

Stores organization branding used for white-label reports and validates active SVG content.

This is an internal developer/AI-agent guide, not customer-facing product copy. Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for cost, data and security invariants.

## Source map

- `handler.go`
- `service.go`
- `repository.go`
- `svg.go`

## API/trigger

GET /api/v1/branding; POST /api/v1/branding; POST /api/v1/branding/reset
