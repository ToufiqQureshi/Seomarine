# MCP server

The Go MCP handler owns the `/mcp` route for Seomarine API-key callers. It
authenticates the existing `oseo_` API key format and serves the complete
registered tool set from Go. Tool discovery and API-key tool calls do not use
the legacy server. Browser/OAuth-authenticated MCP requests still proxy to the
legacy server while the Go OAuth provider is migrated.

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


The Go API-key MCP registry also owns `research_keywords`. It runs each seed independently through the shared Go research service and preserves per-seed failures and result metadata. OAuth requests still use the TypeScript fallback for tools not yet ported.


The Go API-key MCP registry also owns `inspect_urls`. It checks project authorization, uses the saved Search Console property, and returns per-URL inspection outcomes without running a live crawl.


The Go API-key MCP registry also owns `find_serp_competitors`, using the Go domain provider's Labs lookup with market validation, domain exclusions, and the legacy sort choices.


The Go API-key MCP registry also owns `get_local_serp_results`. It queries the Go Maps or Local Finder provider near the supplied coordinate and returns a bounded, trimmed row shape.


The Go API-key MCP registry also owns `get_google_business_questions`. It validates one business identifier and a coordinate scope, then returns trimmed questions and answers from the Go DataForSEO provider.


The Go API-key MCP registry also owns `list_business_categories`. It reads and caches the free provider category index, then applies substring filtering and bounded pagination in memory.


The Go API-key MCP registry also owns `search_local_businesses`. It validates coordinate scope and provider filters, then returns the legacy compact business identity/contact rows.


The Go API-key MCP registry also owns `get_business_profile`, returning the provider profile for exactly one business identifier and the requested project market or coordinate.


The Go API-key MCP registry also owns `get_business_reviews`. It posts Google/extended review tasks once, polls within the request budget, and returns resumable IDs for still-running tasks without repeat charges.


The Go API-key MCP registry also owns `get_business_updates`; it uses the Go task queue path and supports free resume polling with the returned task ID.


The Go API-key MCP registry also owns `get_local_rank_grid`. It uses the Go local SERP provider for bounded Google Maps grids, preserves per-point failures and rank summaries, and computes a market-aware zoom when one is omitted. Contract tests remain deferred to the final test phase.
