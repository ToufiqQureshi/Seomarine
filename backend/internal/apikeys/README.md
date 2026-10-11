# apikeys

Create, list and revoke the API keys people paste into MCP clients. Same routes
and table as Better Auth's api-key plugin (`/api/auth/api-key/create|list|delete`),
so the settings page and keys issued earlier keep working.

- A key is `oseo_` plus 64 random letters. Only `base64url(SHA-256(key))` is
  stored; the plaintext is returned once, by `create`. `start` keeps the first 9
  characters so keys can be told apart in the list.
- `internal/mcp` verifies keys (it uses the same hash). This package never reads
  a plaintext key after creating it.
- Every route needs a session. List and delete are scoped to the caller: another
  user's key id is "not found", never "forbidden".
- A user may hold at most 100 keys, and names are 1 to 32 characters.
- The plugin's own rate limiter stays off; the MCP endpoint enforces a per-user
  limit.
