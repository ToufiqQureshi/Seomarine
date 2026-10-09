# audit

Site audit: crawl a site, run per-page and cross-page checks, optionally sample
Lighthouse through DataForSEO, and serve the results to the React audit pages.
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
- DataForSEO (`DATAFORSEO_API_KEY`) is needed for Lighthouse. Without it audits
  still run; Lighthouse is skipped.

## Rules that must never break

- Every URL the crawler fetches goes through the `Guard` (private, loopback and
  link-local addresses are blocked).
- Every read and delete is scoped by `project_id`. A result from another
  project is "not found", never "forbidden".
- Never send DataForSEO a request it would reject: failed tasks are billed.
- A free plan crawls at most `FreeMaxAuditPages`; paid at most `PaidMaxAuditPages`.

## Deliberate differences from the TypeScript version

- Lighthouse payloads live in Postgres (`payload_json`), not R2. Results list
  rows carry `hasPayload` instead of `r2Key`, and the list query does not load the
  payload.
- Issue details are returned as a parsed `details` object; the React client
  turns it back into the `detailsJson` string its components read.
- Audits are scheduled through the `jobs` queue, not Cloudflare Workflows.

## Edge cases

- A stored payload that is valid JSON but not a version-2 DataForSEO payload is
  treated as "no issue details", not an error. Text that is not JSON is a 500.
- The full Lighthouse export returns the stored text untouched.
- Missing scores sort last (treated as 100) when ranking issues.

## Tests

`go test ./internal/audit/` needs no database for unit and handler tests.
Repository tests need `TEST_DATABASE_URL` and run in CI.
`scripts/audit-mutation-check.py` breaks the code on purpose to prove the tests
notice.
