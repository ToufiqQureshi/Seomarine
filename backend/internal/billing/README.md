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
  for at checkout. Razorpay tells the browser "paid" about a second before
  the webhook arrives, so the Billing screen keeps reading the status (up to
  30 seconds) until the plan changes, instead of reading it once.
- **An unknown `plan_id` is our mistake, not the event's.** It fails with
  `ErrUnknownPlan`, is logged at error level (so it reaches Sentry) and
  answers `500`, so Razorpay retries until the plan id variable is fixed. The
  customer has paid; never answer `200` and drop it.
- **A plan with no Razorpay id cannot be bought**, and the Billing screen says
  so instead of offering a button.
- **Changing plan is not built.** A workspace with a live subscription gets
  `409` on checkout; the screen tells the customer to write to support.
- **One payable subscription per workspace.** Two tabs or a double click get
  the same unpaid subscription; choosing another plan first cancels the unpaid
  one. A second live subscription is cancelled on its first webhook and never
  credited (rules in `backend/README.md` under Billing).

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
it) is what one workspace may cost us at the providers. 1,000 credits are one
US dollar of provider cost before the 1.28 markup, so a credit is 1/1280 of a
dollar of provider cost. Each paid charge of a plan grants that plan's
`MonthlyCredits` (`internal/plans`: Basic 4,000, Pro 10,000, Agency 25,000)
once per webhook event; unused credits roll over up to two months of the
plan (`grantUpToTx`). The test
`TestNoPlanCanCostMoreThanAThirdOfItsPrice` keeps every grant under a third
of the plan's price. `GET /api/v1/billing/credits` returns the plan and
balance, `GET /api/v1/billing/usage` this month's spend by feature and the
typical cost of each action (`costs.go`, `api/billing.yaml`), and `POST
/api/v1/billing/usage-events` lists settled spends.

Plan-wise limits beside credits: sites, tracked keywords, members and the
deepest rank-tracking check (`MaxSerpDepth`: Basic top 30, Pro and Agency
top 40). `TestWeeklyRankTrackingFitsInTheMonthlyCredits` keeps every plan's
tracked keywords, checked weekly at its deepest depth, under 90% of its
monthly credits.

Credits are shown in the app (Billing, and next to paid actions) but not on
the public pricing page; `internal/site` tests keep them off it.

## The spend guard

`SpendGuard` (`spendguard.go`) is the one gate every paid provider request
goes through: it implements `platform/spend.Guard` for the DataForSEO client
(which refuses to start without one) and `AIMeter` for SAM and the writing
assistant. Rules that must never break:

- **Nothing paid is sent without a hold.** Before a DataForSEO request, the
  most it can cost (`dataforseo.EstimateUSD`, from published prices; an
  unknown endpoint is held at one dollar per task) is reserved from the
  workspace's credits. Not enough credits is `spend.ErrNoCredits`, answered
  `402 insufficient_credits`, and the provider is never called. Reservations
  lock the balance row, so parallel requests cannot overspend it.
- **The real cost is charged afterwards**, also for failed tasks the
  provider billed. If it is more than was held and the balance cannot cover
  it, the balance is charged down to zero and a warning is logged: the work
  is already paid for, so it is never refused after the fact.
- **It fails closed.** A ledger or database error stops the request. SAM
  refuses a turn whose credits cannot be checked.
- **Per-workspace daily limit.** One workspace may spend at most a third
  of its plan's monthly credits (in provider dollars) in a UTC day
  (`Service.WorkspaceDailyLimitUSD`); past it, only that workspace gets
  `spend.ErrWorkspaceDailyLimit` (`429 daily_credit_limit`) until midnight
  UTC. One heavy client never pauses anyone else. Staff workspaces have none.
- **Global emergency limit.** Every settled cost, staff included, is logged
  in `go_provider_spend`. Once today's total across all workspaces plus a
  request's estimate passes `PROVIDER_DAILY_LIMIT_USD` (default $100), every
  paid request is refused with `spend.ErrPaused` (`503 usage_paused`) and an
  error reaches Sentry. With per-workspace limits it should only trip on a
  bug; set it above the sum of your clients' daily limits.
- **AI.** A model's cost is only known after it answers, so AI work starts
  only with at least a cent of credit left, is charged after each step, and
  SAM checks again before every step and stops a turn at one dollar. When
  OpenRouter reports no cost, a conservative estimate from tokens (or from
  the streamed text) is charged instead of nothing.
- Without payments (self-hosted) nobody is charged credits, but the daily
  limit still applies.

Edge cases: two requests racing the last credits are decided by the row
lock, so one is refused. Concurrent requests can pass the daily limit by at
most their own estimates, because the daily check is not locked.
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
