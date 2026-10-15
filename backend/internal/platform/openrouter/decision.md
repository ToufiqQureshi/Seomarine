# OpenRouter streaming transport: decisions

> Internal guide for developers and coding agents. Update with code changes.

## Responsibility

Streams OpenAI-compatible chat responses, tool-call deltas and usage from OpenRouter.

## Decisions to preserve

- A request without a token ceiling is refused; callers must reserve a cost ceiling before send. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- Never retry automatically because a retry can bill again. _Why:_ Retries, replays and parallel requests are normal; idempotency prevents duplicate charges, records and side effects.
- Bound SSE/error payloads and preserve partial usage/text for accurate settlement. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.
- If provider cost is absent, use the conservative fallback estimate rather than treating the call as free. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.

## Cost and risk boundary

OpenRouter API, shared HTTP client and caller-owned billing guard.
