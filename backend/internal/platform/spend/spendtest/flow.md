# Spend guard test helper: flow

> Internal test support for developers and coding agents.

## Test flow

1. A unit test injects `spendtest.Guard` in place of an external billing/spend dependency.
2. The feature calls `Hold`; the helper records the request and returns a test hold, or returns the configured refusal.
3. The feature calls `Settle`; the helper records the amount so the test can assert metering behavior.

## Dependencies

This helper has no network or database dependency. It implements the `Guard` contract from `backend/internal/platform/spend` for tests only.

## Safety boundary

Never wire this helper into a server binary: it allows provider work by default and does not enforce real customer or budget limits.
