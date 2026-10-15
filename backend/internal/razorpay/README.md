# Razorpay adapter

Wraps subscription creation/cancellation and webhook signature validation for billing.

This is an internal developer/AI-agent guide, not customer-facing product copy. Read [flow.md](flow.md) for the request path and [decision.md](decision.md) for cost, data and security invariants.

## Source map

- `razorpay.go`

## API/trigger

Called by `backend/internal/billing`; no public direct route.
