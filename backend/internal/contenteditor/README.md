# Content editing and AI writing

Owns project drafts, content briefs, assistant generation, image upload and draft storage.

This is an internal developer/AI-agent guide, not customer-facing product copy. Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for cost, data and security invariants.

## Source map

- `handler.go`
- `brief_handler.go`
- `contenteditor.go`
- `writer.go`
- `images.go`

## API/trigger

POST /api/v1/projects/{projectId}/content/{brief|assistant/generate|drafts/...|images/upload}
