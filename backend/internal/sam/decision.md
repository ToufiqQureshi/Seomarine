# SAM project assistant: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Streams model turns, persists bounded history, invokes registered project tools and meters AI usage.

## Decisions to preserve

- Require a per-turn cost ceiling and stop mid-turn if credits/limits are exhausted. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Never let a model-selected project ID change authorization scope. _Why:_ Callers can tamper with IDs and UI state, so server-verified ownership must prevent cross-customer access.
- Persist partial answers and settle real usage on provider/tool error; avoid charging reported-zero usage as free. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.

## Dependencies

OpenRouter/model provider; Postgres sessions/messages; Redis limits; billing; projectcontext and internal services.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
