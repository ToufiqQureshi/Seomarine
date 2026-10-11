# Authentication sessions

The Go API reads Better Auth's existing `session` table and verifies its
signed session cookie. The hosted `POST /api/auth/sign-out` handler deletes
the matching session immediately and expires both HTTPS and local cookie
names. Missing or invalid cookies still receive a successful sign-out
response, while database failures return a safe error without exposing driver
details.

Sign-up, sign-in, password reset, OAuth and invitation acceptance remain on
the legacy auth route until their Go replacements are complete.
