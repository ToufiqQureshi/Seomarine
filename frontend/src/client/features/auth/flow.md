# Sign-in and account access: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. A visitor opens a public auth route; `AuthPage` validates the form before submitting.
2. Email flows use Better Auth-compatible auth endpoints; Google flows use `useGoogleAuth` and the configured OAuth callback.
3. The server verifies credentials/tokens, applies abuse checks and issues a secure session cookie.
4. Authenticated route guards read the server session; invalid/expired sessions return the user to a public auth route.

## Implementation map

- `AuthPage.tsx`
- `useGoogleAuth.ts`
- `useHostedAuthRouteGuard.ts`
- `backend/internal/emailauth`
- `backend/internal/auth`

## API and service dependencies

Server packages: `backend/internal/emailauth` for credential lifecycle and `backend/internal/auth` for session reading/authorization.

## Cost, security and failure boundaries

- Never store plaintext passwords, verification tokens or OAuth secrets in frontend state or logs.
- Keep origin/callback allowlists, CSRF protections, CAPTCHA/rate limits and secure cookie rules intact.
- A client-side session cache is not an authorization check; every Go API route checks the session.
