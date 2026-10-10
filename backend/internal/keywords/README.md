# Keywords and SERP locations

## Legacy behavior and edge cases found before porting

- Research accepts 1–200 seeds, modes `auto`, `related`, `suggestions`, `ideas`, result limits 150/300/500, optional clickstream and grouping. Auto blends suggestions and ideas, alternating rows and deduplicating keywords; it falls back when fewer than five non-seed rows appear.
- Labs countries use Labs keyword data. Countries marked Google Ads-only use Google Ads, which lacks difficulty and search intent. Local research validates the canonical city/region before spending; local volume replaces national volume rather than silently falling back. National metrics remain available for saved keywords.
- Research and SERP cache keys include organization, project, market, area, seed, mode, depth, and options that change results. A deeper 100-result SERP snapshot answers a 20-result request; an empty deeper response must not evict a useful shallow snapshot.
- Saving normalizes keywords by trim and lowercase, deduplicates, stores supplied metrics, and returns the exact saved row IDs. Tags can append or replace only on those rows; replace requires nonempty tags. Metric rows and tag assignments must remain project-scoped.
- Saved keyword listing supports search, include/exclude terms, volume/CPC/difficulty ranges, tag IDs/names, seven sort fields, pagination at 50/100/250, and an export with the same filters. Rows preserve all nullable metrics, monthly searches, fetched timestamp and tag fields. Malformed monthly-search JSON becomes an empty array.
- Tag assignment/removal supports batches; a tag rename/color edit is project-scoped, and deleting an in-use tag returns the assignment count instead of deleting it. Saved keyword removal accepts at most 2,000 IDs and affects only that project.
- SERP location search accepts a two-letter ISO country and a 1–100 character query, returning at most 50 canonical city/county/municipality/DMA/region rows. Postal codes, states, airports and universities are excluded. Exact first-segment matches rank above prefixes and DMA rows; US/Canada/Australia region abbreviations expand after the place token. A prewarm request fills the same country cache.
- DataForSEO's full per-country registry is large; cache successful, filtered country data for 30 days, coalesce concurrent cold fills, and bound provider calls. A failed or empty provider response must not poison the cache. Invalid input must never reach a billed provider task.

## Go API (all `POST /api/v1/projects/{projectId}/keywords/...`, contract in `api/keywords.yaml`)

`research`, `serp`, `saved/refresh` (billed, paid-plan gated when billing is wired), `saved/save|list|export|remove`, `saved/tags/assign|update|delete`.

- Tables: `go_saved_keywords`, `go_saved_keyword_tags`, `go_saved_keyword_tag_assignments`, `go_keyword_metrics` (migration 00011). They start empty: one-time copy from the legacy tables is an owner step.
- Env: needs `DATAFORSEO_*` for research/serp; Redis for caches (research v5 24h, SERP 12h).
- Never break: validate before any provider call (failed tasks are billed); tag delete locks the tag so a concurrent assign cannot orphan rows; metrics persist under the requested language even when Labs served another.
- Deliberate differences from legacy: NULL metrics sort last in saved lists; research language is normalized to the one Labs serves.

## Migration status

Market tables and resolution are in `platform/market`. Domain analysis now reads the project market server-side. Keyword research, saved keywords, SERP analysis and SERP location API are being ported here; legacy TypeScript remains active until each response contract and caller is switched.


The Go MCP `get_serp_results` tool uses the shared keyword SERP provider for
market and canonical local-location validation. It returns bounded live result
rows per query and captures individual query failures without discarding
successful results. Tool-contract tests are deferred to the final test phase.


The Go MCP `get_keyword_metrics` tool fetches known terms from Labs or Google Ads by market, supports optional clickstream refinement and monthly trends, and returns nullable metrics sorted by the requested field. Its contract tests are deferred to the final test phase.
