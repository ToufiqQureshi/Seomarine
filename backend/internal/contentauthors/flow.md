# Project content authors: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{projectId}/content/authors/{list|save|{authorId}/delete}
- **Source:** `contentauthors.go`

## Request or job flow

Project editor asks to list/save/delete; handler authorizes project; package validates and persists author profile rows for that project; editor and publisher read the saved byline.

## Dependencies and cost boundary

Postgres `go_content_authors`; content editor and site publisher.

## Failure, security and data invariants

- Author profiles are editorial identities, not login accounts.
- Every read/write is project-scoped; validate lengths and fields before persistence.
- Only include author schema fields that the user supplied and the product supports.
