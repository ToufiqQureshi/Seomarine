# WordPress publishing: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Connects a WordPress site, previews article schema and publishes a saved draft after user confirmation.

## Decisions to preserve

- Publishing is a customer-site side effect: never publish from draft autosave, preview or AI generation. _Why:_ These actions affect a customer account, site or external service and must follow an explicit user action with a clear result.
- Retain explicit owner/admin permissions and plan checks. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.
- Sanitize outbound HTML/schema and do not leak WordPress credentials in logs or errors. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- A timeout after WordPress accepts a post must not cause duplicate posts on blind retry. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.

## Verification pointers

- `sitePublishingApi.test.ts`
