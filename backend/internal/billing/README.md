# Billing

Two things live here: the plan a workspace is on, and the internal credit
ledger.

**Plan.** One Pro plan, sold as a Razorpay subscription per workspace
(organization), stored in `go_billing_subscriptions`. `GET
/api/v1/billing/status` reads it, `POST /api/v1/billing/checkout` starts one,
`POST /webhooks/razorpay` keeps it in step with Razorpay. The webhook rules
(which events, replay and ordering safety, which statuses count as Pro) are in
`backend/README.md` under Billing. Without the four `RAZORPAY_*` variables
every billing endpoint answers `503`.

The price is not stored here. It is the amount on the Razorpay plan
`RAZORPAY_PLAN_ID_PRO`, and it must equal `ProPriceINR` in `internal/site` and
`PRO_PRICE_INR` in the frontend.

**Credits.** `CreditLedgerService` (`go_billing_credits` and the tables beside
it) caps what one workspace can cost us at the providers. 1,000 credits are one
US dollar of provider cost, charged with a 1.28 markup. A caller reserves
before it spends, then settles the real amount or refunds. Each paid
subscription charge grants `BILLING_MONTHLY_CREDITS_USD` once per webhook
event. `GET /api/v1/billing/credits` returns the plan and balance, and `POST
/api/v1/billing/usage-events` lists settled spends.

Customers are not meant to see credits (see `ROADMAP.md`, Decided). The old
`/billing` page in the app still shows them; that is an open owner decision.

Never include a provider error body or a credential in a client response.

## Cancelling

`POST /api/v1/billing/cancel` (owner or admin) stops the subscription from
charging again. The public pages promise a one-click cancel, so this must
never need a form, a call or an email.

- A subscription with a paid cycle running (`active`) is cancelled at the end
  of that cycle: the customer keeps Pro until `currentPeriodEnd`, and
  `cancelAtPeriodEnd` is true until Razorpay's `subscription.cancelled`
  webhook arrives.
- Any other live state has no paid time left, so it ends at once. Razorpay
  also refuses an end-of-cycle cancel when no cycle is running.
- Cancelling twice does not call Razorpay again; it would refuse.
- A new checkout clears `cancel_at_period_end`.
