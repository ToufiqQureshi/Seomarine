# Shared provider and runtime clients: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** No direct customer route; mounted feature handlers/jobs call these components.
- **Source:** The folder groups shared Go packages under `dataforseo/`, `entitlements/`, `errreport/`, `httpx/`, `ids/`, `jobs/`, `market/`, `openrouter/`, `pgdb/` and `spend/`.

## Request or job flow

Feature services use the package for their specific concern: provider transport/metering, HTTP bounds, market normalization, Postgres transactions, AI streaming, spend holds or durable jobs. The folder itself is an umbrella; it does not contain runtime code or a customer route.

## Dependencies and cost boundary

HTTP; DataForSEO/OpenRouter/market/spend subpackages; Redis/Postgres queue adapters.

## Failure, security and data invariants

- Provider transport must not bypass feature validation or cost authorization.
- Always set timeout/body limits and close response bodies; do not log secrets.
- Keep metering and queue idempotency consistent across callers.
