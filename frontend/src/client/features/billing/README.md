# Plans, credits and usage

Shows subscription status, plan limits, credit balances and usage; starts checkout or cancellation only after user action.

If a downgrade leaves more active sites, seats or tracked keywords than the
new plan allows, Billing shows the exact counts. Existing data stays intact;
paid provider actions resume after the workspace is brought within the limits.

Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for the rules that protect user data, provider spend and customer trust.

## Code map

- `plan/planApi.ts`
- `billingApi.ts`
- `usageApi.ts`
- `backend/internal/billing`
- `backend/internal/plans`
- `backend/internal/razorpay`
