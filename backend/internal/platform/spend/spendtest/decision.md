# Spend guard test helper: decisions

> Internal test support for developers and coding agents.

## Why it exists

Tests need to prove that a feature requests a spend hold and settles the expected cost without calling a real provider.

## Decisions to preserve

- Keep the guard thread-safe because a test may exercise concurrent service calls. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- Return copies from recorded slices so assertions cannot mutate the helper's internal state. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.
- Keep the helper test-only; its default allow behavior is unsafe for production. _Why:_ Keeping this boundary prevents unrelated code or test helpers from changing production behavior or creating hidden work.
- Add tests that assert both refused calls and successful hold/settlement behavior when changing spend integration. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
