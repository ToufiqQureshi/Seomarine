# Shared HTTP helpers: decisions

> Internal guide for developers and coding agents. Update with code changes.

## Responsibility

Provides bounded JSON decoding, safe pagination, common error envelopes and request middleware.

## Decisions to preserve

- Bound request bodies before decoding and reject duplicate/out-of-range query values. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- Never expose panic details to clients; keep stack traces in protected server logs. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.
- Request IDs must be printable/validated to prevent log injection. _Why:_ User and provider content or URLs can be hostile; validation and escaping prevent unauthorized exposure or executable content.

## Cost and risk boundary

Go net/http and shared handlers.
