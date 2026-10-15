# Local SEO endpoint layer: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{projectId}/local-seo/{businesses|profile|rank-grid}
- **Source:** `handler.go`

## Request or job flow

Handler verifies project and plan, validates coordinate/grid bounds, calls the shared keyword local provider, then returns per-business/per-point outcomes.

## Dependencies and cost boundary

`backend/internal/keywords` provider methods; DataForSEO billing/usage; plan service.

## Failure, security and data invariants

- Run plan, bounds and credit checks before paid provider calls.
- Bound grid size to control worst-case provider spend and request time.
- Preserve individual errors; do not convert provider failures into a rank of zero.
