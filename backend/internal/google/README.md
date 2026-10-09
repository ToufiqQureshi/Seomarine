# Google OAuth

## Legacy behavior and edge cases captured before the port

| Behavior | Required Go behavior |
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

The state store, PKCE and AES-GCM primitives are implemented. The HTTP callback,
legacy account repository, token refresh, disconnect, GSC and GA4 callers are
not yet ported. No React caller uses this package yet.

## Rules

- `go_google_oauth_states` is a short lived state table. Expired rows are swept
  when a state is created. A state is consumed with one `DELETE ... RETURNING`.
- `GOOGLE_TOKEN_ENCRYPTION_KEY` must supply 32 bytes, base64 encoded. The
  ciphertext prefix is a key identifier to support future rotation.
- Never log OAuth state, code, tokens or decrypted ciphertext.
