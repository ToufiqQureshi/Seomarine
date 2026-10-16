# Plans, credits and usage

Shows subscription status, plan limits, credit balances and usage; starts checkout or cancellation only after user action.

Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for the rules that protect user data, provider spend and customer trust.

## Code map

- `plan/planApi.ts`
- `billingApi.ts`
- `usageApi.ts`
- `backend/internal/billing`
- `backend/internal/plans`
- `backend/internal/razorpay`
