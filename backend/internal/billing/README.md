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


## Credit ledger

The credit ledger stores the available balance, one reservation per
organization-scoped idempotency key, and an append-only ledger. Reserving
credits decreases both the balance and ledger by the held amount. Settlement
adds or subtracts only the difference between the hold and actual usage.
If extra usage exceeds the remaining balance, the balance stops at zero and a
separate negative `debt` ledger row records the shortfall. Refunds restore
the full reservation. IDs are generated with `platform/ids.New`.

The database constraints and accounting decisions are documented in
`docs/maintainers/credits-usage-design.md`. DataForSEO and MCP call-level
reserve/settle wiring and the authenticated balance endpoint are still pending;
the ledger service alone does not make those calls billable.
