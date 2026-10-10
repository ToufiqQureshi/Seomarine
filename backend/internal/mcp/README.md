# MCP server

The Go MCP handler owns the `/mcp` route for Seomarine API-key callers. It
authenticates the existing `oseo_` API key format, runs registered Go tools,
and proxies credentials it does not own (including legacy OAuth tokens) to
the TypeScript MCP server. Calls for tools not yet registered in Go are
forwarded to TypeScript with the caller's authorization and protocol headers.

`tools/list` fetches the legacy list and merges it with Go's registry, keeping
the Go definition when a name exists in both servers. If the legacy list is
unavailable or malformed, the request fails rather than quietly hiding tools
from clients. The legacy list is capped at 4 MiB. The API-key request lane has
a 30-second deadline; legacy fallback preserves the caller's credential and
the upstream response.

Postgres stores API-key usage and reserves tables for future OAuth grants; Redis applies the hosted
per-user rate limit. Keep tool names and schemas compatible with cached MCP
clients. When a tool is ported, register its Go handler and remove its
TypeScript duplicate only after contract tests cover the public behavior.
OAuth authorization remains on the TypeScript fallback until the Go OAuth
flow is complete.

The read/free tools currently owned here include saved keyword list/save/remove,
SERP location search, and site audit history/status/pages/deletion. Audit deletion
checks owner/admin membership and uses the same stop-and-delete service as the
dashboard. Tools whose backing Go service or response contract is not ported yet
(including reports, templates/sharing, Search Console, rank tracker reads, and
audit issue formatting) continue through the TypeScript fallback. GA4 report,
overview, measurement-health, and Search Opportunities tools are registered in
Go and call the existing GA4 services.
