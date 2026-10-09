# Google OAuth Go migration

| Area                                   | Status                                         |
| -------------------------------------- | ---------------------------------------------- |
| Behavior inventory                     | Started in `backend/internal/google/README.md` |
| Single-use state and PKCE              | Implemented, pending Postgres integration test |
| AES-GCM token encryption               | Implemented, focused unit test passes          |
| Callback, grant repository and refresh | Not ported                                     |
| GSC and GA4 clients and React callers  | Not ported                                     |

This is a partial, local implementation. Do not route traffic to it until the
remaining OAuth flow and integration tests are complete.
