# Internal link suggestions: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{projectId}/content/link-suggestions
- **Source:** `sources.go`, `linksuggest.go`, `text.go`

## Request or job flow

Editor explicitly requests suggestions; service loads bounded GSC/audit pages, ranks candidates, excludes draft URL, scans text and returns up to ten safe links; Redis caches per project and draft hash.

## Dependencies and cost boundary

Search Console reads, audit tables, Redis cache; no paid SEO provider.

## Failure, security and data invariants

- No lookup while the user is merely typing; this preserves Google quota and latency.
- Only suggest pages from the same authorized project/site; cap input, page and segment counts.
- Redis outage falls back to free computation; source failures return a safe empty/fallback result.
