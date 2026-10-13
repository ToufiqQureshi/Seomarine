# Billing

Two things live here: the plan a workspace is on, and the internal credit
ledger.

**Plan.** Basic, Pro and Agency are sold as Razorpay subscriptions per
workspace (organization), stored in `go_billing_subscriptions`. What each one
costs, limits and includes is in `internal/plans`; this package only answers
"which plan is this workspace on" and "does that plan include X".

`GET /api/v1/billing/status` reads the plan, `GET /api/v1/billing/plans`
returns the catalog for the Billing screen, `POST /api/v1/billing/checkout`
(`{"plan": "basic" | "pro" | "agency"}`) starts a subscription, and
`POST /webhooks/razorpay` keeps it in step with Razorpay. The webhook rules
(which events, replay and ordering safety, which statuses keep a plan on) are
in `backend/README.md` under Billing.

Rules that must never break:

- **The plan comes from Razorpay.** A webhook carries the Razorpay `plan_id`
  that was paid for, and that decides the plan, never what the browser asked
  for at checkout.
- **An unknown `plan_id` is our mistake, not the event's.** It fails with
  `ErrUnknownPlan`, is logged at error level (so it reaches Sentry) and
  answers `500`, so Razorpay retries until the plan id variable is fixed. The
  customer has paid; never answer `200` and drop it.
- **A plan with no Razorpay id cannot be bought**, and the Billing screen says
  so instead of offering a button.
- **Changing plan is not built.** A workspace with a live subscription gets
  `409` on checkout; the screen tells the customer to write to support.

**Checking a plan.** A feature package asks one question through
`Service.Gate(plans.<Feature>)`, and limits come from `SiteLimit`,
`KeywordLimit` and `MemberLimit` (0 means no limit). On a server without
Razorpay the gates are not wired at all, so features are open; the site and
member limits still apply, because they are read from the stored plan.

**Staff workspaces.** `STAFF_EMAILS` lists people whose own workspaces are
never limited: a workspace whose **owner** has one of those **verified**
addresses reads as Agency with `unlimited: true`, has no limits, and is never
refused for credits (it is topped up, and the spend is still in the ledger).
Being a member of someone else's workspace unlocks nothing there.

**Credits.** `CreditLedgerService` (`go_billing_credits` and the tables beside
it) caps what one workspace can cost us at the providers. 1,000 credits are one
US dollar of provider cost, charged with a 1.28 markup. A caller reserves
before it spends, then settles the real amount or refunds. Each paid
Pro charge grants `BILLING_MONTHLY_CREDITS_USD` and each Agency charge
`BILLING_AGENCY_MONTHLY_CREDITS_USD`, once per webhook event; Basic grants
none, because it includes nothing that spends credits. `GET /api/v1/billing/credits` returns the plan and balance, and `POST
/api/v1/billing/usage-events` lists settled spends.

Customers are not meant to see credits (see `ROADMAP.md`, Decided). The old
`/billing` page in the app still shows them; that is an open owner decision.

Never include a provider error body or a credential in a client response.

## Without Razorpay credentials

The server still serves subscription status and credit balance from Postgres.
Responses include `paymentsConfigured: false` so the client can hide payment
actions. Checkout, cancellation and webhooks return 503. Paid-feature gates
retain their existing self-hosted behavior until payments are configured.
If an existing subscriber needs to cancel while Razorpay is unavailable,
support must arrange cancellation in Razorpay; restore credentials promptly.

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
