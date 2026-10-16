# Content editing and AI writing: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Owns project drafts, content briefs, assistant generation, image upload and draft storage.

## Decisions to preserve

- Do not call paid providers on page load or keystrokes; cache and explicit user actions control spend. _Why:_ Passive viewing and editing should not create paid work or external side effects without the user asking for them.
- Apply output/input/image bounds and do not publish generated content automatically. _Why:_ These actions affect a customer account, site or external service and must follow an explicit user action with a clear result.
- Project authorization and draft IDs must be checked together; do not trust the caller’s project field alone. _Why:_ This keeps the behavior explicit so future changes do not silently alter customer-visible results.

## Dependencies

Postgres drafts/brief cache/authors; configured AI/DataForSEO providers; object storage for images; billing.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
