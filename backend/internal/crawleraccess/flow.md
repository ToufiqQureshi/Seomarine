# Protected-site crawler credentials: flow

> Internal guide for developers and coding agents. Verify current behavior against source and tests before changing it.

## Entry points

- **API/trigger:** POST /api/v1/projects/{projectId}/crawler-access/save; GET credentials; POST delete
- **Source:** `handler.go`, `service.go`

## Request or job flow

Owner submits a project credential; service validates format, encrypts before storing and lists only masked metadata; the audit crawler resolves credentials only for the matching project/host.

## Dependencies and cost boundary

Postgres credential rows; `BETTER_AUTH_SECRET`-derived encryption; audit crawler.

## Failure, security and data invariants

- Plaintext exists only for the request/needed crawl; never return it from list or log it.
- Fail closed on malformed ciphertext or missing/weak encryption configuration.
- Bind the credential to the expected project/site so it cannot be sent to an attacker-controlled redirect.
