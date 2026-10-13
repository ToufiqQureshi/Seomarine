# Google OAuth

## Behavior and edge cases

| Case | Required behavior |
|---|---|
| Consent supports GSC and GA4 with separate provider IDs, callback paths and scopes. | Bind each state to one provider and session user. |
| Callback URL may be external or protocol relative. | Reduce it to a same origin path, defaulting to `/`. |
| State expires after ten minutes and is consumed atomically. | Reject expiration, replay and a mismatched user; a failed callback cannot reuse a state. |
| PKCE is S256; the verifier derives from state, provider and client secret. | Generate a secure random state and a deterministic provider bound verifier. |
| Google may omit a refresh token on renewed consent. | Preserve the existing refresh token. |
| Access tokens refresh within five seconds of expiration. | Serialize refresh per account and persist a rotated refresh token. |
| Multiple users may authorize the same Google account. | Scope grants by user, provider and Google account ID. |
| Disconnect removes matching project mappings and grant atomically. | Preserve other users' grants and social login. |
| OAuth errors return to the initiating page with provider marker. | Invalid state returns to `/auth-error`. |

## Status

Everything in the table is implemented: consent start
(`POST /api/v1/google/oauth/{provider}/start`), the callbacks
(`/api/gsc/oauth/callback`, `/api/ga4/oauth/callback`), grant storage, token
refresh, and account removal (`POST /api/v1/google/accounts/impact` and
`/remove`). `internal/gsc` and `internal/ga4` call Google through this
package's API client. Sign-in with Google is separate: it lives in
`internal/emailauth` and stores no Google tokens.

## Rules

- `go_google_oauth_states` is a short lived state table. Expired rows are swept
  when a state is created. A state is consumed with one `DELETE ... RETURNING`.
- `GOOGLE_TOKEN_ENCRYPTION_KEY` is optional. When set it must supply 32
  bytes, base64 encoded. When empty, the key is derived from
  `BETTER_AUTH_SECRET`, which is why that secret must never change on a live
  site. The ciphertext prefix is a key identifier to support rotation.
- `golang.org/x/crypto/chacha20poly1305` is needed solely to read Better Auth's
  existing XChaCha20-Poly1305 grants. Go's standard library lacks XChaCha.
- Versioned Better Auth ciphertext envelopes need the matching old key ring;
  a bare `BETTER_AUTH_SECRET` only decrypts bare hexadecimal ciphertext.
- Never log OAuth state, code, tokens or decrypted ciphertext.
