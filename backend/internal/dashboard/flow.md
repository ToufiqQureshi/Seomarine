# Dashboard response contracts: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** No standalone route; consumed by `dashboardoverview` handlers and the dashboard client.
- **Source:** `types.go`

## Request or job flow

Dashboard UI requests `dashboardoverview`; response types in this package keep card fields consistent; activation state is separately provided by `activation`.

## Dependencies and cost boundary

Go package contracts only; no IO, provider calls or persistence.

## Failure, security and data invariants

- Keep this package free of side effects and provider dependencies.
- Change JSON fields only with corresponding client and contract-test updates.
- Do not implement refresh behavior in a response-types package.
