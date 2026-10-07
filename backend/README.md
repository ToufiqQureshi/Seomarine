# Seomarine backend (Go)

## Run

```sh
DATABASE_URL=postgres://user:pass@localhost:5432/seomarine PORT=8080 go run ./cmd/server
```

- `GET /healthz`: liveness. The process is up.
- `GET /readyz`: readiness. Postgres answers within 2 seconds, otherwise 503.

The server fails at startup if `DATABASE_URL` is missing or unreachable, and
shuts down gracefully on SIGINT or SIGTERM.

## Checks (all mandatory, see /CLAUDE.md)

```sh
go mod tidy && gofmt -l . && go vet ./... && staticcheck ./... \
  && golangci-lint run ./... && go test -race -count=1 -cover ./... \
  && govulncheck ./... && deadcode ./...
```

Postgres integration tests need `TEST_DATABASE_URL`. CI always sets it, and
they fail there if it's missing.
