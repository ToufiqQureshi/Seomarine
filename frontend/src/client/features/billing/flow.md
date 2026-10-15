# Plans, credits and usage: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. Billing routes load the plan catalog, subscription status, balances and usage through `plan/planApi.ts`, `billingApi.ts` and `usageApi.ts`.
2. The user explicitly chooses a plan or cancellation; the frontend sends the matching mutation.
3. Go billing validates the catalog plan and provider state, then calls the Razorpay adapter when configured.
4. Webhook events are verified and idempotently applied; usage and credit reservations are shown from server records.

## Implementation map

- `plan/planApi.ts`
- `billingApi.ts`
- `usageApi.ts`
- `backend/internal/billing`
- `backend/internal/plans`
- `backend/internal/razorpay`

## API and service dependencies

Go packages: `billing`, `plans` and `razorpay`; payment actions happen only after explicit user input.

## Cost, security and failure boundaries

- The server catalog is the source of truth for price, entitlement and limits; never duplicate these rules in UI code.
- Credit reservations must settle/refund safely on provider failure and duplicate requests.
- Never trust a browser-reported price, plan or provider cost.
- Checkout/cancel are externally visible billing actions; require explicit user intent and preserve idempotency.
