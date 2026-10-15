# DataForSEO transport: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** No direct route; called by domain, keywords, backlinks, AI visibility, rank tracking, audit and local SEO.
- **Source:** `*.go in this package; see README.md`

## Request or job flow

Caller validates inputs and ownership; spend guard reserves/attributes the task; transport sends bounded request with timeout; response/task status is parsed; usage is persisted for the responsible organization.

## Dependencies and cost boundary

DataForSEO API, platform spend guard, Postgres usage ledger, Redis worker/cache as caller requires.

## Failure, security and data invariants

- Provider may bill rejected/failed tasks: validate request before transport and never add automatic retries to task-post calls.
- Attribute every billable task to the organization causing it; usage-write failure behavior must remain explicit.
- Never put credentials in URLs/logs; cap body/response sizes and enforce timeouts.
