# Protected-site crawler credentials: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Encrypts and resolves per-project credentials used when crawling protected customer sites.

## Decisions to preserve

- Plaintext exists only for the request/needed crawl; never return it from list or log it. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- Fail closed on malformed ciphertext or missing/weak encryption configuration. _Why:_ Unavailable or partial data is not a measured zero; keeping status visible prevents false customer decisions.
- Bind the credential to the expected project/site so it cannot be sent to an attacker-controlled redirect. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.

## Dependencies

Postgres credential rows; `BETTER_AUTH_SECRET`-derived encryption; audit crawler.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
