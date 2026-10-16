# Lighthouse issue details: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Presents Lighthouse and Core Web Vitals data already collected by a site audit.

## Decisions to preserve

- Viewing stored details must not trigger another PageSpeed/CrUX request. _Why:_ Passive viewing and editing should not create paid work or external side effects without the user asking for them.
- Keep unavailable distinct from insufficient traffic data. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.
- Do not calculate or rewrite Google metrics from presentation code. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.

## Verification pointers

- `issues/utils.test.ts`
