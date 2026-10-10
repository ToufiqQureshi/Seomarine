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
