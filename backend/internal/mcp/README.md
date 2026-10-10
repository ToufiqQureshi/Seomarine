# MCP server

The Go MCP handler owns `/mcp` for Seomarine API-key callers. It authenticates
the existing `oseo_` key format, runs registered Go tools, and proxies
authorizations and tools it has not ported to the TypeScript MCP server.
`tools/list` merges the Go registry with the legacy list and keeps Go definitions
when a name exists in both.

Project context read/write tools use the shared Go `projectcontext` service and
the same project authorization boundary as the HTTP API. Updates are atomic and
apply the same limits and canonicalization used by the settings API. Other
unported tools continue through the TypeScript fallback.

Keep public names, schemas, annotations and response shapes compatible with
cached clients. Remove TypeScript MCP tools only after Go contract tests cover
their public behavior. OAuth authorization remains on the legacy path until the
Go OAuth flow is complete.
