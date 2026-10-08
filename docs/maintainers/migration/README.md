# TypeScript to Go migration

This directory coordinates the migration described in [FEATURES.md](FEATURES.md).
Every product capability must remain available through the React client and Go
API. `STATUS.md` is the current file-level checklist; a row stays TODO until Go
serves the behavior, the client uses that endpoint, parity tests pass, and the
legacy TypeScript server implementation is deleted in the same change.

## Owners and lanes

| Owner | Scope                                                                                                                                         |
| ----- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| Codex | Shared `backend/internal/platform`, identity, projects, project-context, domain, serp-locations, keywords, rank-tracking, and rank workflows. |
| Buffy | Audit, Lighthouse, crawler access, Google integrations and reports, billing, AI search, SAM, backlinks, and growth/admin features.            |

`TASK-CODEX.md` and `TASK-BUFFY.md` define each worker's order. Only edit your
assigned feature and its progress file. Platform code belongs to the foundation
work and must be coordinated before feature lanes depend on it.

## Change rules

- One mini-task per branch and one draft pull request. Branch from the latest
  `main`, rebase before opening the PR, and run the relevant Go and frontend
  checks. Do not push feature work directly to `main`.
- Keep HTTP handlers thin: handler, service, repository. Prefer the standard
  library and PostgreSQL with `pgx`; avoid wrappers and unused abstractions.
- Use existing legacy tables without changing their schema. New Go-owned tables
  use the `go_` prefix and migrations are additive.
- Scope repository queries to the authenticated organization. Add a test that
  proves another organization cannot read the data.
- Port the legacy behavior and tests before deleting TypeScript. Generated
  files must come from their generator, not manual edits.
- Never edit another owner's rows, commit secrets, or remove either license.

## Per-feature progress

Use [TEMPLATE.md](TEMPLATE.md) for a feature's progress note. Keep the file list
and line counts synchronized with `STATUS.md` until that inventory is split
into feature-level records. Never mark a feature DONE based only on a platform
foundation package.

## Completion checks

Record each command and result in the PR: `gofmt`, `go vet ./...`,
`go test -race -cover ./...`, `go mod tidy`, `staticcheck`, `golangci-lint`,
`govulncheck`, `deadcode`, `pnpm ci:check`, and `pnpm test`. State why any check
could not run. Include the TypeScript lines removed, Go lines added, parity
tests, tenant-isolation test, and remaining work.
