# Backlinks

## Legacy behavior and edge cases captured before the port

| Behavior in TypeScript and its tests | Go behavior / edge case to keep |
|---|---|
| Overview fetches the DataForSEO summary and one year of daily history for domain scopes. | Exact page scope omits history. History dates cover UTC yesterday back one calendar year; both provider tasks fail the request if either task fails. |
| A subfolder cannot be targeted directly by summary/history. | Two sequential backlinks list tasks count all links and one-per-domain links with a `url_to` prefix filter; rank, spam, broken/new/lost values and trends stay null/empty. |
| Target scopes include exact URL, subfolder, domain and subdomains; persisted `page` is an alias for exact URL. | Normalize host through IDNA, remove leading `www.`, reject IPs, invalid suffixes, credentials and exact URLs with queries/fragments. A subfolder needs a non-root path. |
| Backlinks rows support all-links or strongest-per-domain mode, sort, pagination, spam hiding and include/exclude/numeric/link/lost/broken/domain filters. | Strict sort mapping, `50/100/200` page sizes, a maximum of eight provider filter conditions including scope and spam filters; include terms OR, excludes AND, escaped LIKE metacharacters. |
| Referring domains have their own aggregate filters and keep spam rows visible in the UI. | No spam clause is added. Subfolder scope returns the same validation message because this endpoint has no target-page field. |
| Top pages support pagination, sort and include/exclude/count/rank filters. | Subfolder scope is expressed as URL-prefix filters; the combined expression may not exceed eight conditions. |
| Summary, tabs and provider rows preserve nullable values and map provider spelling variants. | Keep nulls; map `new_reffering_domains` and `lost_reffering_domains`, `attributes`, legacy spam-score field, `lost_date`, and `last_visited` fallbacks. Empty row arrays serialize as `[]`. |
| Data is shared across callers and cached for six hours, including target scope, path, organization and all page/filter parameters. | Redis is only an optimization; cache read/write failures log and fall through. Org ID is part of every key. Provider calls are never retried. |
| Billing classification distinguishes an account balance problem from ordinary errors. | Plan check runs before cache access or paid provider calls; free plan receives 402 when billing is configured; without billing, endpoints are open. Provider billing issue returns 503. |

## Flow

`requireSession -> requireProjectAccess -> paid-plan check -> handler validation -> service/cache -> platform/dataforseo`

Routes are `POST /api/v1/projects/{projectId}/backlinks/{overview,rows,referring-domains,top-pages}`. There is no repository or table: the feature is a read-only provider proxy and uses no tenant-owned data. Project authorization supplies the organization ID that is used for plan checks, DataForSEO usage recording and Redis key isolation.

## Rules that must not break

- Never call DataForSEO before validating target, scope, filters, sort and pagination. Provider tasks are billed even on failure.
- `Client.Do(..., false)` is mandatory: a replay can charge the organization twice.
- Cache entries are organization-scoped; cache failures must not turn a valid provider call into an error.
- DataForSEO summary/history are whole-target endpoints. Subfolder counts must stay filtered and cannot claim unavailable rank/trend data.
- UI response fields and nullability match the TypeScript schemas. New API contracts are validated with Zod.
- The legacy MCP tools still call `BacklinksService`; that TypeScript service and its provider helpers remain until MCP is ported.

## Configuration, limits and tests

`DATAFORSEO_API_KEY` configures the shared provider transport. No new environment variables or tables are required. Redis caches successful responses for six hours. Request bodies are limited to 32 KiB, filter strings to 500 bytes and provider filter expressions to eight conditions. Provider outbound timeouts and usage recording are enforced by `platform/dataforseo`.

Tests cover target normalization, filter translation and budget, handler validation/plan checks, result mapping, provider errors, cache isolation/outages and cancellation. No database tenant-isolation test applies because this package has no repository; project authorization and organization-attributed provider usage are covered at the HTTP boundary.
