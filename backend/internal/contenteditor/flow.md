# Content editing and AI writing: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{projectId}/content/{brief|assistant/generate|drafts/...|images/upload}
- **Source:** `handler.go`, `brief_handler.go`, `contenteditor.go`, `writer.go`, `images.go`

## Request or job flow

The editor submits one explicit operation; handler verifies project and body bounds; brief/writer logic optionally calls configured providers and applies credit/plan checks; repository stores draft/brief cache; image path validates upload before storage.

## Dependencies and cost boundary

Postgres drafts/brief cache/authors; configured AI/DataForSEO providers; object storage for images; billing.

## Failure, security and data invariants

- Do not call paid providers on page load or keystrokes; cache and explicit user actions control spend.
- Apply output/input/image bounds and do not publish generated content automatically.
- Project authorization and draft IDs must be checked together; do not trust the caller’s project field alone.
