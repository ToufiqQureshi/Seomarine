# HTTP routing and middleware

Mounts public site, auth, API routes, shared session/project guards, tracker collection and SPA fallback.

This is an internal developer/AI-agent guide, not customer-facing product copy. Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for cost, data and security invariants.

## Source map

- `httpapi.go`
- `spa.go`
- `tracker.js`

## API/trigger

All routes are registered here or by feature `Mount` functions.
