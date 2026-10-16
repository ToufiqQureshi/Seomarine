# Ahrefs domain ratings: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{projectId}/backlinks/domain-ratings
- **Source:** `handler.go`, `service.go`

## Request or job flow

Handler validates project and request; service normalizes/deduplicates domains and performs one bounded request to the public Ahrefs rating endpoint; response keys are mapped back to original inputs.

## Dependencies and cost boundary

Ahrefs public domain-rating endpoint; no product database writes.

## Failure, security and data invariants

- Keep batch size and host validation before outbound calls.
- Do not use or describe this endpoint as paid backlink detail; detailed backlink reports use DataForSEO in `backlinks`.
- Never log provider credentials or return raw transport errors.
