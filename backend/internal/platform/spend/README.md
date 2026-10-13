# spend

The contract between code that costs Uplumo money at a provider and the
credit ledger that pays for it. `billing.SpendGuard` implements it; see
`internal/billing/README.md` for the rules (holds, settling, the daily
limit, AI).

- A provider transport asks `Guard.Hold` before it sends billed work, with
  the most the work can cost, and calls `Hold.Settle` exactly once with the
  real cost, also when the work failed.
- `Feature` labels the spend so the Billing screen can show where credits
  went. `Reference` is free lookup data (location lists, model lists).
- A refusal is `ErrNoCredits` or `ErrPaused`. Handlers call
  `WriteHTTPError` (402 `insufficient_credits`, 503 `usage_paused`) before their
  own mapping; MCP and rank tracking use `Message`. The DataForSEO client also
  wraps refusals in its `ErrBillingIssue`, so a handler that forgets the
  mapping still stops and answers 503 instead of 500.
- `spendtest.Guard` allows everything and records holds, for tests only.
