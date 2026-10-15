# Razorpay adapter: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** Called by `backend/internal/billing`; no public direct route.
- **Source:** `razorpay.go`

## Request or job flow

Billing service validates plan and intent; adapter sends bounded API request with credentials; webhook signatures are verified before billing mutates subscription state.

## Dependencies and cost boundary

Razorpay HTTP API and secret configuration.

## Failure, security and data invariants

- No raw payment secret in URL, logs or error responses.
- Use deadlines and validate signatures with constant-time comparison.
- Never let the adapter decide plan price or entitlement; billing catalog remains authoritative.
