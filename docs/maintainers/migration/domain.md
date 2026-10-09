# Domain Go migration

| Area            | Status     | Notes                                                                                            |
| --------------- | ---------- | ------------------------------------------------------------------------------------------------ |
| Go API          | Done       | `overview`, `keyword-suggestions`, `keywords`, `pages` in `backend/internal/domain`.             |
| React callers   | Done       | Domain page, search tabs and the rank-tracking keyword step call the Go API via `domainApi.ts`.  |
| Server function | Removed    | `src/serverFunctions/domain.ts` and its four request schemas are deleted.                        |
| Legacy service  | Retained   | `DomainService` and its helpers stay: the MCP tools still import them. Remove with the MCP port. |
| Database        | Not needed | Provider-backed with a Redis cache; no tables.                                                   |

## Deliberate differences from TypeScript

- The client sends the resolved market (`domainMarket.ts`); see the package README.
- Scope filter slots are counted from the clauses instead of hard-coded, and a
  test pins them to the UI budget.
- Keyword rows keep the ranking `url` and `relativeUrl` (the table links to them).

## Validation

Go: `go test -race -cover ./internal/domain/` (94% statements, 20 hand-made
mutations all caught). The end-to-end test in `internal/httpapi` needs Postgres
and Redis and runs in CI.
