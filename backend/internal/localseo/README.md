# Local SEO endpoint layer

Validates and orchestrates local business lookup/profile and bounded Google Maps rank grids.

This is an internal developer/AI-agent guide, not customer-facing product copy. Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for cost, data and security invariants.

## Source map

- `handler.go`

## API/trigger

POST /api/v1/projects/{projectId}/local-seo/{businesses|profile|rank-grid}
