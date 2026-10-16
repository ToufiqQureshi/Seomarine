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
`KeywordLimit` and `MemberLimit` (0 means no limit). On a live server without
Razorpay, the read-only billing service still gates features from stored
subscription status. Self-hosted development without Razorpay leaves feature
gates open; site and member limits still apply from the stored plan.
The MCP dispatcher applies the same catalog to every registered tool before
calling its handler, including SAM's in-process tool runner. New MCP tools
must be classified explicitly; unknown tools fail closed when billing is on.
Local SEO research routes use the Keywords gate as well.

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
plan: each grant expires after two months and reservations consume the oldest
unexpired grants first. The hourly expiry worker records `expire` ledger rows;
balance reads and reservations also expire that workspace's overdue grants.
The test
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

After a downgrade, existing sites, members and tracked keywords are retained.
`Service.UsageAgainstPlan` reports the overage to Billing, and the shared
`SpendGuard` refuses new paid provider work with `plan_limits_exceeded` until
the workspace fits the current catalog limits. Reads, archiving and removal
remain available so the customer can resolve the overage without data loss.

Monthly credits are shown on the public pricing page beside each plan's
included tools and limits. The app shows the balance in Billing and estimated
costs next to paid actions. `internal/site` reads the grant from the plan
catalog, so pricing cannot drift from the amount awarded after payment.

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
- **The real successful-task cost is charged afterwards.** Failed DataForSEO
  tasks stay in provider spend telemetry but refund the customer's task
  credits. If successful work costs more than was held and the balance cannot cover
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
- **AI.** A model's cost is only known after it answers, so every AI call
  holds its ceiling first (`AIMeter.Hold` with `openrouter.MaxCostUSD`: the
  input counted at three bytes a token plus the whole output budget, at the
  expensive-end fallback prices of $5 / $25 per million tokens) and settles
  the real cost after. A SAM step with 16,000 output tokens holds about $0.40
  plus its input (about 640 credits for a typical chat); a writing action
  about $0.07 (90 credits). The held credits come back at settle, so they
  only decide whether the call may start: a workspace with fewer credits than
  the ceiling is refused instead of being let in and charged down to zero,
  which was Uplumo's money. SAM stops a turn at one dollar. When OpenRouter
  reports no cost, a conservative estimate from tokens (or from the streamed
  text) is charged instead of nothing.
- Without payments (self-hosted) nobody is charged credits, but the daily
  limit still applies.

Edge cases: two requests racing the last credits are decided by the row
lock, so one is refused. Both daily limits count today's open holds at their
ceilings as well as settled spend. A workspace's daily check and its
reservation run in one transaction under a per-workspace advisory lock, so
its parallel requests cannot together pass its limit, and no workspace waits
on another's. The global emergency limit is not locked across workspaces, so
requests in different workspaces can pass it by at most their own estimates.
A hold left open by a crash counts until midnight UTC.
Never include a provider error body or a credential in a client response.

## Without Razorpay credentials

The server still serves subscription status and credit balance from Postgres.
Responses include `paymentsConfigured: false` so the client can hide payment
actions. Checkout, cancellation and webhooks return 503. Live deployments
keep plan gates active from stored subscription status; local development
retains the self-hosted open-gate behavior.
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
