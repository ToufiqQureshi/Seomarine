# MCP API keys: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** GET /api/auth/api-key/list; POST /api/auth/api-key/create; POST /api/auth/api-key/delete
- **Source:** `apikeys.go`

## Request or job flow

Authenticated settings request creates a random key; only its hash and short display prefix are stored; the raw key is returned once. MCP authenticates by hash; deletion is caller-scoped.

## Dependencies and cost boundary

Better Auth-compatible key store; `backend/internal/mcp` verifier.

## Failure, security and data invariants

- Never persist or log plaintext keys.
- Show the full key only in the create response and keep the 100-key/name bounds.
- A foreign key ID should look absent, not reveal another user’s credential.
