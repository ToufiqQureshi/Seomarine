# MCP server

The Go MCP handler owns the `/mcp` route for Seomarine API-key callers. It
authenticates the existing `oseo_` API key format, runs registered Go tools,
and proxies OAuth credentials and tools that are not ported yet to the legacy
server. `tools/list` merges both registries and keeps Go definitions on name
collisions.

The Go registry includes the account `whoami` tool. Hosted mode comes from
`AUTH_MODE`, and the tool reads the active organization's Autumn usage and
top-up balances without charging credits. If Autumn is unavailable or has no
balance, the response reports the balance as unknown. Self-hosted mode does
not call Autumn.

Keep tool names and JSON schemas compatible with MCP clients. OAuth
authorization and unported tools still use the TypeScript fallback and are
tracked as remaining migration work. Tests for migrated tool contracts are
deferred to the final migration test phase.
