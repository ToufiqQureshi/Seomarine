# Ahrefs domain ratings

Looks up free domain-rating values for bounded batches of normalized hosts.

This is an internal developer/AI-agent guide, not customer-facing product copy. Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for cost, data and security invariants.

## Source map

- `handler.go`
- `service.go`

## API/trigger

POST /api/v1/projects/{projectId}/backlinks/domain-ratings
