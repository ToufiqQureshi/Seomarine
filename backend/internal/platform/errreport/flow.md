# Error reporting transport: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** No route; invoked by error-reporting middleware/call sites.
- **Source:** `errreport.go`

## Request or job flow

Caller provides error/context; sanitizer drops sensitive fields; configured client sends bounded event; missing config disables reporting without blocking product work.

## Dependencies and cost boundary

Sentry endpoint/configuration; shared HTTP client.

## Failure, security and data invariants

- Never report passwords, tokens, request bodies containing user content or provider credentials.
- Reporting failure must not fail the original user request or recursively report itself.
- Keep environment/config opt-in and bounded.
