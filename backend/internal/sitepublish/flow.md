# Customer-site publishing: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{projectId}/site-connection/...; POST .../content/drafts/{draftId}/publish
- **Source:** `handler.go`, `sitepublish.go`, `wordpress.go`, `schema.go`

## Request or job flow

Owner/admin saves a WordPress connection; user asks for schema preview or publish; handler checks project, plan, draft and permissions; publisher sends bounded WordPress request; publication result is recorded.

## Dependencies and cost boundary

WordPress REST API; Postgres site connection/publication/draft rows; plan catalog.

## Failure, security and data invariants

- Publish is an external side effect; preview and draft save must never publish.
- Do not retry uncertain create-post calls blindly; use persisted result/idempotency safeguards.
- Keep credentials encrypted/redacted and sanitize content/schema before sending.
