# Plans, credits and usage: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Shows subscription status, plan limits, credit balances and usage; starts checkout or cancellation only after user action.

## Decisions to preserve

- The server catalog is the source of truth for price, entitlement and limits; never duplicate these rules in UI code. _Why:_ A single server-owned source keeps UI access, checkout and limits aligned even if client state is stale or modified.
- Credit reservations must settle/refund safely on provider failure and duplicate requests. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- Never trust a browser-reported price, plan or provider cost. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Checkout/cancel are externally visible billing actions; require explicit user intent and preserve idempotency. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.

## Verification pointers

- `credits/CreditCostHint.test.ts`
- `credits/CreditsCard.test.ts`
