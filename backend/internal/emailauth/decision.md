# Email and social authentication: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when implementation behavior or invariants change.

## Responsibility

Implements signup, sign-in, email verification, password reset/profile update and Google callback compatibility.

## Decisions to preserve

- Keep password hashing, token expiry/one-time use, cookie attributes and callback allowlists intact. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- Rate-limit account and email operations before expensive or abuse-prone side effects. _Why:_ This work consumes paid quota or shared capacity, so authorization and bounds must be checked before dispatch.
- Do not reveal whether an account exists in recovery/verification responses. _Why:_ Explicit failure behavior helps callers recover safely instead of treating an error as a valid result.

## Dependencies

Better Auth tables/session reader; Loops; Google OAuth; CAPTCHA provider.

## Verification

Use the package tests next to the source files in `README.md`. For changes crossing an API boundary, also check the mounted route/contract and the caller feature’s `flow.md`.
