# Email and social authentication: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/auth/sign-up/email; sign-in; verification/reset endpoints; GET OAuth callback
- **Source:** `handler.go`, `service.go`, `password.go`, `token.go`, `google.go`

## Request or job flow

Request is origin/rate/CAPTCHA checked; passwords are hashed; one-time tokens are stored/validated; transactional email is sent; successful authentication creates a Better Auth-compatible session cookie.

## Dependencies and cost boundary

Better Auth tables/session reader; Loops; Google OAuth; CAPTCHA provider.

## Failure, security and data invariants

- Keep password hashing, token expiry/one-time use, cookie attributes and callback allowlists intact.
- Rate-limit account and email operations before expensive or abuse-prone side effects.
- Do not reveal whether an account exists in recovery/verification responses.
