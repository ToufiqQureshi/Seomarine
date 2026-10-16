# Keyword and local business research: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{projectId}/keywords/...; POST /api/v1/serp-locations/{search|prewarm}
- **Source:** `research_handler.go`, `research.go`, `saved_handler.go`, `saved.go`, `location_search.go`

## Request or job flow

Handler validates seeds, market, options and limits; service chooses supported DataForSEO endpoint; paid calls are gated/metered; results normalize/cache; save/list/tag/export operations use project-scoped Postgres.

## Dependencies and cost boundary

DataForSEO; Google Ads for selected markets; Postgres saved rows; Redis caches; platform market registry.

## Failure, security and data invariants

- Validate every billed task before provider call; no blind retries of paid task-post endpoints.
- Cache keys include project/organization, geography and all result-changing options.
- Keep saved keyword rows and tags project-scoped; guard batches/caps and null metrics.
