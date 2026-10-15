# Search Console connection prompts: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Contains Search Console re-engagement UI; actual OAuth, property selection and reports live in Integrations/Search Performance.

## Decisions to preserve

- The modal is not the source of OAuth token or property state; keep ownership in `integrations` and Go `google`/`gsc`. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- Do not imply a property is connected until the server confirms it. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.
- Google API reads are quota-limited even though they do not use paid SEO credits. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.

## Verification pointers

- Verify the listed source files and the feature route manually when behavior changes.
