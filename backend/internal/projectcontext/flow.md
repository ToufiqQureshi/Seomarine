# Project context memory: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{projectId}/context/{get|update}
- **Source:** `handler.go`, `service.go`, `repository.go`, `context.go`

## Request or job flow

UI or agent reads project context; update request is canonicalized/validated; service applies full batch transactionally under project row lock; reads render stable typed context and bounded research history.

## Dependencies and cost boundary

Postgres project context rows; `projects` authorization; SAM/MCP consumers.

## Failure, security and data invariants

- Keep project-scoped auth and atomic updates; concurrent writes must preserve caps/uniqueness.
- Bound prompt memory (counts, text length and history window) to control model cost and avoid unbounded storage.
- Omitted fields retain values only where documented; do not accidentally clear user data.
