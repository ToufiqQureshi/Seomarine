# Billing

The Go billing package owns Razorpay subscriptions and the hosted Autumn
integrations used by the Go API and MCP tools. Autumn credit balance reads use
the active organization as customer ID and never modify usage. Provider
failures are kept separate from a genuine zero balance so callers can report
an unknown balance safely.

Autumn's feature IDs are shared with the frontend contract: `usage_credits`
and `topup_credits`. Keep API secrets in runtime configuration only; never
include provider error bodies or credentials in client responses. Webhook
status synchronization and the remaining billing surface are still migrating.

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
