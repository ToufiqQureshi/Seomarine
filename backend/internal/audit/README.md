# audit

Site audit: crawl a site, run per-page and cross-page checks, optionally sample
Lighthouse through Google's PageSpeed Insights API, and serve the results to the
React audit pages.
Ported from the TypeScript engine; the JSON contract is unchanged so the pages
only changed their transport.

## Flow

1. `POST /projects/{id}/audit/start` validates the URL (SSRF guard), checks plan
   limits and audit capacity, inserts a `go_audits` row, and enqueues a job on the
   `jobs` queue (`AuditQueueName`).
2. The worker started in `cmd/server/main.go` claims the job and the `Runner`
   runs it: discovery (robots.txt, sitemaps), the crawl, the checks, then the
   Lighthouse phase. Progress goes to Redis (live feed) and `go_audits` (counters).
3. The pages poll `status`, `progress` and `results`. Lighthouse detail pages
   use `lighthouse/issues` and `lighthouse/export`.

Jobs are at-least-once. Row ids are deterministic, so a retried job rewrites the
same rows instead of duplicating them.

## Endpoints

All are `POST /api/v1/projects/{projectId}/audit/<name>`, need a session and
membership of the project's organization, and take a JSON body (unknown fields
are rejected): `start`, `status`, `results`, `history`, `progress`, `delete`,
`capabilities`, `lighthouse/issues`, `lighthouse/export`. Spec:
`backend/api/audit.yaml`.

## Tables

`go_audits`, `go_audit_pages`, `go_audit_issues`, `go_audit_lighthouse_results`
(migration `00007_audit.sql`). Pages, issues and results cascade with the audit.

## Env vars

- `AUDIT_BROWSER_RENDERING=true` or `CONTEXT_API_KEY` enables JavaScript
  rendering. Without either, `renderJavaScript` requests answer 403.
- `PAGESPEED_API_KEY` (a Google Cloud API key with the PageSpeed Insights API
  enabled) is needed for Lighthouse. Without it audits still run; Lighthouse is
  skipped.

## Lighthouse through PageSpeed Insights

Google runs Lighthouse on its own machines, so a check costs no money and no
credits. Running Lighthouse ourselves would need Chrome in the image and about
half a gigabyte of memory per check on the shared server. DataForSEO ran the
same Lighthouse for a fee.

- The one cost is quota: 25,000 checks a day for the whole Google Cloud project,
  shared by every workspace. So each workspace may run
  `PageSpeedDailyChecksPerOrganization` (200) checks per UTC day, counted in
  Redis. Over that, the check fails with a message saying when it resets; the
  rest of the audit still runs. If Redis is down the check fails closed.
- A free plan's "auto" becomes "homepage": 2 checks (mobile and desktop). Paid
  plans sample the homepage plus one page per URL template, up to 10 pages, 20
  checks.
- Each response also carries Google's real-visitor data (CrUX, last 28 days),
  stored as `fieldData`: the page's own data, or the whole site's when the page
  has too few visits, or nothing for a low-traffic site. "Core Web Vitals
  passed" follows PageSpeed Insights: LCP and CLS good, and INP good when
  measured.
- The key goes in the `X-Goog-Api-Key` header, never the URL: a transport error
  repeats the URL, and that message is stored on the audit for the customer.
- A page Google could not load (`NO_FCP`, `ERRORED_DOCUMENT_REQUEST`) is the
  customer's site, so it is logged as a warning, not an error.

## Rules that must never break

- Every URL the crawler fetches goes through the `Guard` (private, loopback and
  link-local addresses are blocked).
- Every read and delete is scoped by `project_id`. A result from another
  project is "not found", never "forbidden".
- One workspace's page speed checks never use up another's: the daily cap is
  per organization.
- A free plan crawls at most `FreeMaxAuditPages`; paid at most `PaidMaxAuditPages`.

## Deliberate differences from the TypeScript version

- Lighthouse payloads live in Postgres (`payload_json`), not R2. Results list
  rows carry `hasPayload` instead of `r2Key`, and the list query does not load the
  payload.
- Issue details are returned as a parsed `details` object; the React client
  turns it back into the `detailsJson` string its components read.
- Audits are scheduled through the `jobs` queue, not Cloudflare Workflows.

## Edge cases

- A stored payload that is valid JSON but not a version-2 payload (source
  `pagespeed-insights`, or `dataforseo-lighthouse` from before the switch) is
  treated as "no issue details", not an error. Text that is not JSON is a 500.
- The full Lighthouse export returns the stored text untouched.
- Missing scores sort last (treated as 100) when ranking issues.

## Tests

`go test ./internal/audit/` needs no database for unit and handler tests.
Repository tests need `TEST_DATABASE_URL` and run in CI.
`scripts/audit-mutation-check.py` breaks the code on purpose to prove the tests
notice.
