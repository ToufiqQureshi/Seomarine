# Domain overview

Domain Overview page: organic traffic and keyword count for a domain, plus two
tabs (ranking keywords, top pages) and the keyword suggestions used by rank
tracking. Ported from the TypeScript `DomainService`.

## Endpoints

All are `POST /api/v1/projects/{projectId}/domain/<name>` and need a session
and membership of the project's organization.

| Name                  | Returns                                                        |
| --------------------- | -------------------------------------------------------------- |
| `overview`            | traffic, keywords, `hasData`, echoed `scope` and label         |
| `keyword-suggestions` | top 100 ranking keywords by traffic (no paging)                |
| `keywords`            | one page of ranking keywords, with ranking `url`/`relativeUrl` |
| `pages`               | one page of ranking pages                                      |

Errors: `400` bad input, `402` free plan (only when billing is configured),
`503` no `DATAFORSEO_API_KEY` or a provider balance problem, `502` any other
provider failure.

## Flow

`session -> project access -> plan gate -> decode + validate -> service -> cache -> DataForSEO Labs`

The plan gate runs before the body is read, so a free plan never costs a task.
Validation runs before the provider: failed provider tasks are billed.

## Rules that must never break

- **Validate before spending.** Host, scope, paging, sort and filters are
  checked in the handler or service; an invalid request reaches the provider
  zero times (tests assert the call count).
- **Filter budget = 8.** The scope consumes slots (keywords: exact URL 4,
  subfolder 4, domain 1; pages: 4/4/2) and the search box costs 2. The slot
  numbers must equal `RESEARCH_SCOPE_FILTER_SLOTS` in
  `src/shared/researchScope.ts`, which the UI uses to shrink its own budget.
  Counting is done by `countConditions`, not hard-coded.
- **Cache keys carry organization and project**, plus every input that changes
  the answer (scope, path, market, page, size, sort, filters, search).
- Empty overviews and empty suggestions are never cached (a later lookup must
  retry); a cached entry without data is ignored.
- Redis down is soft: log and call the provider.

## Differences from the TypeScript version

- **The server resolves the market.** A project-scoped SQL lookup reads
  `projects.location_code` and `language_code` for the authorized organization.
  A location override selects that country's default language unless the request
  also supplies a language. An unsupported project default falls back to US/English.
  Explicit unserved Labs locations and language pairs are rejected before billing.
- Request schemas moved from Zod on the server to Go validation; the response
  contract is unchanged and checked by Zod in `domainApi.ts`.
- The Playwright fixtures (`VITE_E2E_DOMAIN_FIXTURES`) now short-circuit in
  `domainApi.ts` instead of the server function.

## Tables, env

Reads the legacy `projects` table, filtered by both project and organization.
No new tables. Needs `DATAFORSEO_API_KEY`; Redis is optional.

## Edge cases covered by tests

`www.` stripping, IPs and fake TLDs rejected, path kept for subfolder/exact URL,
LIKE wildcards escaped in filter text, UTF-16 length limits (emoji), rows with
no keyword or no address dropped, relative URL derived from the ranking URL,
`hasMore` with and without a provider total.
