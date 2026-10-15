# Domain overview and research: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{projectId}/domain/...
- **Source:** `handler.go`, `provider.go`, `repository.go`, `filters.go`

## Request or job flow

Project request resolves the default market; handler rejects invalid scopes/filters and checks access before provider work; provider code calls DataForSEO and response filters are applied to normalized results.

## Dependencies and cost boundary

DataForSEO paid endpoints, project market and plan checks.

## Failure, security and data invariants

- Validate target, market and filter budget before spend; failed provider tasks may still be billed.
- Keep scope semantics explicit: domain, subdomain, subfolder and exact URL do not support identical aggregates.
- Escape user filters for SQL/provider syntax and cap response size.
