# Site audit: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Why the feature is structured this way

Starts an asynchronous crawl and presents technical SEO findings, crawl progress and optional performance data.

## Decisions to preserve

- **Audit start can consume shared crawl capacity and Google quota** — keep per-plan limits, per-organization quotas and timeouts. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- **Every crawl URL must pass the SSRF guard** — never let redirect targets bypass it. _Why:_ User and provider content or URLs can be hostile; validation and escaping prevent unauthorized exposure or executable content.
- **Lighthouse/CrUX unavailability is a partial result, not proof that a page has no field data** _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.
- **At-least-once jobs must remain idempotent so retries do not duplicate audit rows** _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- Compare only completed audits sharing the same start URL, and describe absent crawl rows as missing rather than deleted. _Why:_ Different crawl scopes are not comparable, and crawler discovery gaps can resemble customer-side removals.
- Load saved result payloads only after the user requests a comparison; keep comparison local and read-only. _Why:_ History browsing should not create extra crawl/provider work or mutate customer data.
- Compute orphan suggestions lazily from saved audit data, cap displayed orphans and candidate links, and keep the user in control of adding links. _Why:_ Bounded local work avoids repeated processing and prevents an inferred suggestion from changing a customer site without review.

## Verification pointers

- `auditApi.test.ts`
