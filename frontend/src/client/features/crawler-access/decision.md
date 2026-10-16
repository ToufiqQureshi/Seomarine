# Crawler access settings: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Feature purpose

Stores and manages customer-provided crawler access credentials for protected sites.

## Decisions to preserve

- Never return plaintext credentials after save or include them in logs/errors. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- Encryption key rotation and malformed ciphertext handling must stay fail-closed. _Why:_ Explicit failure behavior helps callers recover safely instead of treating an error as a valid result.
- Do not send stored credentials to an unrelated host or project. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.

## Verification pointers

- `crawlerAccessApi.test.ts`
