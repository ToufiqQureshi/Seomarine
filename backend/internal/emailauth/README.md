# emailauth

Email and password accounts: sign-up, sign-in, email verification, password
reset and profile update. It speaks Better Auth's HTTP contract over Better
Auth's own tables, so the React client (`authClient`) and every existing
account keep working. Only hosted mode mounts these routes.

## Flow

- `POST /api/auth/sign-up/email`: creates the user and a credential account in
  one transaction, emails a verification link, signs nobody in. An address that
  already has an account gets the same answer as a new one (and costs the same
  hashing time), so the endpoint cannot list registered emails.
- `GET /api/auth/verify-email`: checks the HS256 link, marks the address
  verified and starts a session unless the request already carries it, then
  redirects to `callbackURL` (`?error=CODE` on a bad link).
- `POST /api/auth/sign-in/email`: password first, then the verified-email check,
  so a wrong password never reveals that an account exists but is unverified.
- `POST /api/auth/send-verification-email`, `request-password-reset`,
  `GET reset-password/{token}`, `POST reset-password`, `POST update-user`.

A session is created inside the same transaction that picks the workspace it
lands in (last active, else newest membership, else a new personal workspace;
nothing is minted for someone with a pending invitation).

## Rules that must never break

- Password hashes stay `"<salt hex>:<scrypt key hex>"`: N=16384, r=16, p=1,
  64 bytes, salt used as the hex text, password NFKC-normalized. Changing any of
  it locks existing users out. Tests verify hashes made by Better Auth itself.
- Session cookies are `better-auth.session_token` (`__Secure-` prefix on https),
  value `encodeURIComponent(token.base64(HMAC-SHA256))`. The Go session reader in
  `internal/auth` accepts what this package writes.
- Only HS256 tokens are accepted, with a mandatory expiry.
- A reset or verification token is single-use: the reset row is deleted with
  `DELETE ... RETURNING`, so two racing requests cannot both win.
- Redirect targets (`callbackURL`, `redirectTo`) must be a path on this site or
  a trusted origin; `//host`, `/\host` and encoded forms are refused.
- A state-changing request that carries cookies must come from a trusted origin.
- Scrypt costs ~32 MiB, so at most 4 run at once.
- Internal errors never reach the client; provider text is logged.

## Env vars

`BETTER_AUTH_SECRET`, `BETTER_AUTH_URL` (public origin), `LOOPS_API_KEY`,
`LOOPS_TRANSACTIONAL_VERIFY_EMAIL_ID`, `LOOPS_TRANSACTIONAL_RESET_PASSWORD_ID`,
`TURNSTILE_SECRET_KEY` (turns on the sign-up captcha), `BYPASS_EMAIL_VERIFICATION`
(development only). Without Loops, verification and reset answer "not enabled".

## Deliberate differences from Better Auth

- A failed password-reset email is logged and still answered with the generic
  success, instead of an error that would reveal the address exists.
- Rate limits are Redis fixed windows per IP and route (sign-up 5/min, sign-in
  10/min, verification and reset emails 3/min); a Redis outage fails open.
- Referral (Dub) capture on sign-up is not ported yet.
- Not here yet: Google sign-in, organization invitations, API keys. Those stay
  on the legacy route.
