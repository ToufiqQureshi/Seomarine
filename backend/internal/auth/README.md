# Authentication sessions

The Go API reads Better Auth's existing `session` table and verifies its
signed session cookie. The hosted `GET /api/auth/get-session` endpoint returns
the Better Auth session and user JSON contract, including the configured
`analyticsOptedOut` field, or JSON `null` for a missing, invalid, revoked or
expired session. It does not trust the client-side cached session cookie.
After Better Auth's one-day update age, it extends the session by seven days
and refreshes the matching secure or local cookie.

The hosted `POST /api/auth/sign-out` handler deletes the matching session
immediately and expires both HTTPS and local cookie names. Missing or invalid
cookies still receive a successful sign-out response, while database failures
return a safe error without exposing driver details.

Sign-up, sign-in, verification, password reset and Google sign-in live in
`internal/emailauth`; API keys in `internal/apikeys`.

`ba_organization.go` serves the organization plugin routes the client calls
(`/api/auth/organization/get-invitation`, `accept-invitation`, `reject-invitation`,
`cancel-invitation`, `remove-member`, `set-active`) with Better Auth's JSON.
Rules: an invitation is usable only while pending, unexpired and addressed to the
caller's email; accepting creates the membership, points the session and the next
sign-in at that workspace, and skips first-run onboarding. Only owners and admins
may cancel invitations or remove members, an owner can never be removed (ownership
moves through the transfer endpoint), and removing a member clears their active
workspace. Creating or deleting organizations and sending invitations are
deliberately not here: each user gets one workspace at sign-in, and the team API
sends invitations.
