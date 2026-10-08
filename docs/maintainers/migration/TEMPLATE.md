# <feature>

Owner: <Codex or Buffy>  
Go package: `backend/internal/<feature>`  
Status: TODO

## Behavior and endpoints

- Legacy behavior to preserve:
- Go routes and methods:
- Client integration:

## Source files

| TypeScript file | Lines | Status |
| --------------- | ----: | ------ |
| `src/...`       |     0 | TODO   |

## Parity and isolation

- TypeScript tests ported to Go:
- Tenant-isolation test and organization boundary:
- External API fixtures, timeout, and error cases:

## Changes

- TypeScript lines deleted:
- Go lines added:
- Pull request:

## Verification

| Check                        | Result |
| ---------------------------- | ------ |
| `gofmt`                      |        |
| `go vet ./...`               |        |
| `go test -race -cover ./...` |        |
| `go mod tidy`                |        |
| `staticcheck`                |        |
| `golangci-lint`              |        |
| `govulncheck`                |        |
| `deadcode`                   |        |
| `pnpm ci:check`              |        |
| `pnpm test`                  |        |

## Remaining work and risks

-
