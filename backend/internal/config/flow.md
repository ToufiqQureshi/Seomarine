# Runtime configuration: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** Read during server startup; no direct route.
- **Source:** `config.go`

## Request or job flow

Startup parses environment values into typed config; invalid required settings stop startup or disable the dependent optional service explicitly; consumers receive injected config.

## Dependencies and cost boundary

Environment variables and deployment secret store.

## Failure, security and data invariants

- Never commit secrets or include their values in logs/errors.
- Distinguish required core settings from optional provider keys so an unavailable paid feature does not take down the whole app.
- Validate URL/key formats once at startup and inject config rather than reading global env throughout handlers.
