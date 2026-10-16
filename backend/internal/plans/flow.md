# Plan catalog: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** Consumed by billing/site and feature services; no own HTTP route.
- **Source:** `catalog.go`

## Request or job flow

Services request the immutable catalog; public pricing and billing derive display values; feature packages ask `Allows` before restricted use.

## Dependencies and cost boundary

Go catalog and Razorpay plan configuration.

## Failure, security and data invariants

- Never duplicate price/limits in frontend or feature code.
- Every advertised feature must be enforced, and Razorpay amount/plan ID must match the catalog.
- Plan limit changes need cost/unit-economics review and test coverage.
