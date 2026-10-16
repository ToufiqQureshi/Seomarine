# HTTP routing and middleware: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** All routes are registered here or by feature `Mount` functions.
- **Source:** `httpapi.go`, `spa.go`, `tracker.js`

## Request or job flow

Server constructs dependencies; router mounts public/authenticated handlers; session/project middleware wraps API groups; JSON/error helpers apply contracts; unmatched routes return safe 404 or SPA.

## Dependencies and cost boundary

Every feature handler, auth middleware, logging and dependency wiring.

## Failure, security and data invariants

- Public routes must be intentionally public; every private API path uses the correct session/project/role guard.
- Do not bypass middleware when adding a route or mount the same route twice.
- Keep errors free of secrets/driver details and preserve request deadlines/cancellation.
