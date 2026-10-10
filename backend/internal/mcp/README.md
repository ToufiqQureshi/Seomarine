# MCP server

The Go MCP handler owns the `/mcp` route for Seomarine API-key callers. It
authenticates the existing `oseo_` API key format, runs registered Go tools,
and proxies OAuth credentials and tools that are not ported yet to the legacy
server. `tools/list` merges both registries and keeps Go definitions on name
collisions.

The Go API-key lane includes the account `whoami` tool and the `run_site_audit` tool. Audit starts reuse Go's project authorization, plan/capacity checks, SSRF guard, background job queue, and optional Lighthouse/rendering controls. Hosted mode comes from
`AUTH_MODE`, and the Go tool reads the active organization's Autumn usage and
top-up balances without charging credits. If Autumn is unavailable or has no
balance, the response reports the balance as unknown. Self-hosted mode does
not call Autumn. OAuth requests still use the TypeScript fallback, so its
matching tool remains registered until the Go OAuth flow replaces that path.

Keep tool names and JSON schemas compatible with MCP clients. OAuth
authorization and unported tools still use the TypeScript fallback and are
tracked as remaining migration work. Tests for migrated tool contracts are
deferred to the final migration test phase.


The Go API-key MCP registry also owns `get_backlinks_overview`. It reads the
organization-scoped Go backlinks service, applies the default spam filter to
the top referring-domain rows, and preserves the scope notes and provider
limitations from the legacy tool. OAuth calls still use the TypeScript fallback.


The Go API-key MCP registry owns `get_backlinks_profile`, including bounded
pagination, sort mapping, spam filtering, grouping, and source/target filters.
The Go service applies the same target validation and filter budget as its HTTP
API. OAuth calls still use the TypeScript fallback.


The Go API-key MCP registry owns `get_domain_overview`. It uses the Go
Domain service for organic estimates and the Go backlinks service for backlink
totals, while resolving country/language from the project when omitted.


The Go API-key MCP registry owns `get_domain_keyword_suggestions`, using the
Go Domain service, project market defaults, and supported Labs location and
language validation.


The Go API-key MCP registry also owns `get_ranked_keywords`. It uses the Go Domain service for scoped, filtered, market-specific ranked keyword rows and preserves the legacy sorting and pagination contract. OAuth requests still use the TypeScript fallback for tools not yet ported.


The Go API-key MCP registry also owns `get_serp_results`. Each query resolves
its own market and optional canonical local location, while individual provider
errors are returned alongside successful query rows. OAuth requests still use
the TypeScript fallback for tools not yet ported.


The Go API-key MCP registry also owns `create_rank_tracker`. It creates an empty
configuration with the project's market/domain defaults, validates optional
canonical local locations, and keeps the MCP default schedule manual so tracker
creation never starts future credit spend.


The Go API-key MCP registry also owns `estimate_rank_tracker_cost`. It reads
the project-scoped tracker and calculates live and scheduled estimates without
starting a check or spending provider credits.


The Go API-key MCP registry also owns `run_rank_tracker`. It requires an
explicit positive credit ceiling, rechecks the live estimate in the Go check
service, and queues the run through the shared background jobs path. Active runs
return their blocking run ID without creating another run.


The Go API-key MCP registry also owns `get_keyword_metrics`. It resolves the project market, selects Labs or Google Ads, supports optional clickstream refinement and monthly trend omission, and preserves nullable metric fields. OAuth calls still use the TypeScript fallback for tools not yet ported.
