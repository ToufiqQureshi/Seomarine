# Google OAuth and account grants: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/google/oauth/{provider}/start; account impact/remove routes
- **Source:** `oauth.go`, `oauth_handler.go`, `tokens.go`, `crypto.go`, `accounts.go`

## Request or job flow

OAuth start creates state/PKCE; callback validates state and exchanges code; token cipher stores credentials; Google API wrapper refreshes once after 401; owner can preview and remove grants atomically.

## Dependencies and cost boundary

Google OAuth/token endpoints; encrypted token storage; GA4/GSC consumers.

## Failure, security and data invariants

- Validate state, PKCE, exact redirect/callback origins and requested scopes.
- Keep refresh tokens encrypted; never retry a non-idempotent Google request after 401.
- Account removal impact and owner check must happen before destructive deletes.
