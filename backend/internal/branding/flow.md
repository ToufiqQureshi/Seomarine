# Report branding: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** GET /api/v1/branding; POST /api/v1/branding; POST /api/v1/branding/reset
- **Source:** `handler.go`, `service.go`, `repository.go`, `svg.go`

## Request or job flow

Owner/admin request loads or saves organization branding; service checks plan/role, validates fields and sanitizes SVG; repository stores the organization record; reports consume it during render.

## Dependencies and cost boundary

Postgres organization branding; reports renderer; plan catalog.

## Failure, security and data invariants

- Keep organization scoping and role/plan checks server-side.
- Reject executable or externally referenced SVG content; do not relax the sanitizer to accept a logo.
- Public report output must escape text and only render validated branding.
