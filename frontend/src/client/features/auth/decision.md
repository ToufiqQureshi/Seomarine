# Sign-in and account access: decisions

> Internal guide for developers and coding agents. Update this file in the same PR when a behavior or invariant changes.

## Why the feature is structured this way

Frontend sign-up, sign-in, verification, password recovery, Google sign-in and hosted-route guards.

## Decisions to preserve

- **Never store plaintext passwords, verification tokens or OAuth secrets in frontend state or logs** _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- **Keep origin/callback allowlists, CSRF protections, CAPTCHA/rate limits and secure cookie rules intact** _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.
- **A client-side session cache is not an authorization check** — every Go API route checks the session. _Why:_ These values or checks protect account access; exposing them or weakening validation can enable account or provider abuse.

## Verification pointers

- `passwordSchema.test.ts`
