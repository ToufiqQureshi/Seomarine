# aisearch

AI search lookups for a project: **Brand Lookup** (how often ChatGPT and Google
AI Overviews mention a brand or domain, which pages they cite, how it compares
with competitors) and **Prompt Explorer** (one prompt asked of ChatGPT, Claude,
Gemini and Perplexity, answers side by side). Ported from the legacy
`src/server/features/ai-search`. Both are stateless: no tables of their own,
Redis cache only.

## Flow

```
POST /api/v1/projects/{projectId}/ai-search/brand-lookup
POST /api/v1/projects/{projectId}/ai-search/prompt-explorer
  httpapi: requireSession -> requireProjectAccess (puts the project's organization in the context)
  handler.go   decode, validate, plan check, map errors to HTTP
  service.go   cache, fan-out to the provider, partial-failure rules
  provider.go  DataForSEO AI optimization endpoints (through platform/dataforseo)
  shape.go     provider rows -> the result the page renders (pure functions)
  prompt.go    answer text, citations, brand highlight (pure functions)
  models.go    which model name to ask for (live catalog, pins, fallbacks)
  target.go    query -> domain or keyword; research scope; URL safety
```

Request and response JSON is byte-compatible with the legacy server functions
(`src/types/schemas/ai-search.ts` validates it on the client), so the pages did
not change shape.

## Rules that must not break

- **Only the project's organization pays.** Every provider call carries the
  organization from `auth.ProjectOrganizationFromContext`, never the session's
  active organization, and `platform/dataforseo` writes one `go_dataforseo_usage`
  row per billed task. A handler without it fails closed (500).
- **Paid plan only, when billing exists.** `Deps.Plans` (the billing service)
  gates both endpoints with 402 before any provider spend. With no Razorpay
  configured there is no plan to check and the endpoints are open.
- **Never send the provider a request it rejects.** DataForSEO bills failed
  tasks. So: scopes and countries are validated first, `model_name` is checked
  against the provider's free catalog, `web_search_country_iso_code` is dropped
  when web search is off, `force_web_search` goes to Claude only, and domains
  must end in a real public suffix.
- **No automatic retries of paid calls** (`retrySafe=false`). The one deliberate
  retry: a model that was allowed to search but answered from memory is asked
  once more; the retry replaces the answer only if it actually searched.
- **Partial failure is data.** A failed platform shows as `status: "error"` in
  its row; a failed sub-call falls back to empty data. Only
  `dataforseo.ErrBillingIssue` (and a canceled request) fails the whole call,
  and it stops at the first failing call.
- **Never cache a degraded result.** Brand lookups are cached 24 h only when
  every call succeeded and there is data; prompt answers 7 days, successes only.
- **Untrusted URLs.** Citations come from LLM answers. Only `http(s)` URLs without
  credentials are returned (`safeHTTPURL`), because the page renders them as links.
- **ChatGPT is US/English only.** It is always queried with 2840/`en` and left out
  of totals, trends and share of voice for any other locale.
- **URL scopes are post-filtered.** The provider cannot target a URL, so
  `exact_url` and `subfolder` filter page rows afterwards; totals, trends and
  share of voice stay domain-wide and the result says so
  (`aggregatesAreDomainLevel`).

## Cache

Redis, keys `ai-search:<kind>:<organizationId>:<digest>`. The digest covers the
organization, project, target, competitors (sorted, lowercased), locale, scope
and path (path only under URL scopes); for prompts the model, resolved model
name, whitespace-collapsed prompt (case kept), web search and country. The
highlight brand is deliberately **not** in the key: it is re-applied on every
read. The organization is in the clear so everything it cached can be deleted by
prefix. **Privacy erasure:** cached prompts can hold personal data, and the
TypeScript erasure job does not know this Redis namespace yet. Entries expire
within 7 days; the job must `SCAN`/`UNLINK` `ai-search:*:<organizationId>:*` when
it is ported.

## Configuration

| Variable             | Meaning                                                                                                      |
| -------------------- | ------------------------------------------------------------------------------------------------------------ |
| `DATAFORSEO_API_KEY` | Base64 `login:password` of the DataForSEO account. Unset: both endpoints answer 503 and the server still runs. |

Provider spend is in `go_dataforseo_usage` (migration 00006), per organization.

## Gotchas

- Limits are UTF-16 units, like the web app's schemas, so `truncate` and
  `utf16Len` count units and never split a character.
- `mentionsBrand` reimplements the legacy regex without lookaround (Go's RE2 has
  none): word boundaries are required only on sides where the brand ends in a
  word character, otherwise the side must not repeat the brand's edge character
  (`C++` does not match `C+++`).
- Model names: the catalog is cached for an hour; an outage falls back to a
  snapshot list and is not cached. ChatGPT is pinned to `gpt-5.6-luna` while the
  catalog lists it.
- A country that a model does not support is reported per model
  (`UNSUPPORTED_COUNTRY`) without calling it.

## Tests

`go test ./internal/aisearch/` needs `TEST_REDIS_URL` (and uses an in-process
fake DataForSEO). The end-to-end test in `internal/httpapi` also needs
`TEST_DATABASE_URL` and proves authorization and billing attribution.
