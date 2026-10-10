# Credits and DataForSEO Usage Design Note

## Overview
This design note details the architecture for credit metering, DataForSEO request billing, and usage ledger persistence in Seomarine as part of the Go migration.

## Billing Principles & Mechanics

### Reserve -> Settle / Refund Pattern
External providers (e.g., DataForSEO, LLMs) charge on per-request or per-unit basis. To prevent double charges or unbilled consumption:
1. **Reserve**: Before making an outbound API call, check balance and reserve estimated max credits in Redis/DB using atomic decrements.
2. **Execute**: Make the outbound API call with input validation. Never send invalid payloads (DataForSEO bills even for failed task executions if accepted by their gateway).
3. **Settle**: On HTTP 200 / successful provider completion, calculate exact provider cost × markup, record the usage row in `go_dataforseo_usage`, and settle the reservation.
4. **Refund**: On provider error, timeout, or validation rejection, release the reservation immediately so the user is not charged.

### Legacy Numerical Constants
- **Markup Multiplier**: `1.28` (28% margin over raw DataForSEO cost).
- **Credit Value Rate**: `1000 credits = $1.00 USD` (1 credit = $0.001 USD).

### Architectural Transition: Autumn -> Razorpay + Go Ledger
- **Legacy System**: Used Autumn billing / credit API.
- **Go Target**: Replaced by Razorpay subscription plans (granting monthly quota credits) combined with an idempotent PostgreSQL transaction ledger: `go_dataforseo_usage`.

## Owner Decisions & Configuration Flags
- `CREDIT_MARKUP_MULTIPLIER`: Default `1.28`. Owner can adjust margin.
- `CREDITS_PER_USD`: Default `1000`.
- `ALLOW_CREDIT_OVERAGE`: Default `false`. If false, requests fail with 402 Payment Required when credit quota is exhausted.

## Validation & Fraud Protection
- Pre-validate all DataForSEO parameters (domain format, location IDs, language codes, keyword array length) on the Go server before calling outbound API endpoints. DataForSEO charges for any task that passes initial gateway schema validation even if downstream execution fails.
