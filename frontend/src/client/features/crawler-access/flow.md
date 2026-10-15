# Crawler access settings: flow

> Internal guide for developers and coding agents. Confirm current behavior in the linked implementation and tests before changing it.

## User and request flow

1. User opens the crawler access form and submits a credential for a project.
2. `crawlerAccessApi.ts` sends save/list/delete actions to the Go routes.
3. Go validates project membership, encrypts credentials before persistence and returns redacted credential metadata.
4. The audit crawler resolves a credential only when it needs access to the matching protected site.

## Implementation map

- `CrawlerAccessForm.tsx`
- `crawlerAccessApi.ts`
- `backend/internal/crawleraccess/service.go`
- `backend/internal/audit/crawleraccess.go`

## API and service dependencies

Go package: `backend/internal/crawleraccess`; consumer: `backend/internal/audit`.

## Cost, security and failure boundaries

- Never return plaintext credentials after save or include them in logs/errors.
- Encryption key rotation and malformed ciphertext handling must stay fail-closed.
- Do not send stored credentials to an unrelated host or project.
