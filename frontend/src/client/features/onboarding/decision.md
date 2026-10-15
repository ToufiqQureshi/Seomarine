# First-run onboarding: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Guides a new user through website/project setup and optional Search Console connection.

## Decisions to preserve

- Do not mark a step complete until its server mutation succeeds. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.
- Onboarding and OAuth are separate states; a skipped Google connection must not block the product. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- Repeated saves and dismissals should remain idempotent. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.

## Verification pointers

- `onboardingModel.test.ts`
