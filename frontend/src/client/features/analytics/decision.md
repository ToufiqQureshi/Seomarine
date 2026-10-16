# Uplumo Analytics: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Why the feature is structured this way

A cookie-free first-party tracker and project analytics dashboard.

## Decisions to preserve

- **Do not add cookies or persist raw IP addresses** — visitor IDs are daily salted hashes. _Why:_ Collecting or retaining less identifying data reduces privacy exposure if logs, storage or exports are accessed improperly.
- **Collection is public by design, so preserve bot/domain checks and per-IP/per-site rate limits** _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- **Custom events must use the same validation and rate limits as pageviews** _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- **This feature does not call paid SEO providers** _Why:_ Passive viewing and editing should not create paid work or external side effects without the user asking for them.

## Verification pointers

- `analyticsApi.test.ts`
- `presentation.test.ts`
