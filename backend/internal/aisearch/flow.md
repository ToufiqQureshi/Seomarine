# AI Visibility and Prompt Explorer: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{projectId}/ai-search/brand-lookup; POST /api/v1/projects/{projectId}/ai-search/prompt-explorer
- **Source:** `handler.go`, `service.go`, `provider.go`, `shape.go`, `prompt.go`

## Request or job flow

Session/project middleware identifies the owning organization; handler validates and checks plan; service applies cache and bounded provider fan-out; provider uses DataForSEO; pure shaping code produces the React contract.

## Dependencies and cost boundary

Redis cache; DataForSEO AI endpoints; plans/billing.

## Monthly Brand Lookup history

`POST /api/v1/projects/{projectId}/ai-search/brand-lookup/history` uses the same session, project and paid-plan guards as Brand Lookup. It validates the same lookup inputs, derives the organization/project/target/competitor/market/scope Redis key and returns stored aggregates only; it never calls DataForSEO. `BrandLookup` saves a single `YYYY-MM` snapshot only after every platform/cross-brand call completes and the result contains data. Redis reads/writes are best effort: read failure logs and returns empty history without failing the current lookup. The hash keeps at most 13 months, removes older fields and expires after 400 days.

## Failure, security and data invariants

- Plan and request validation must happen before provider spend.
- Keep all billed calls attributed to the project organization; never blindly retry paid POSTs.
- Cache only successful complete results; preserve per-platform partial failures and safe citation URLs.
- A cached prompt may contain user-entered text; retain expiry and privacy-erasure limitations in the feature README.
