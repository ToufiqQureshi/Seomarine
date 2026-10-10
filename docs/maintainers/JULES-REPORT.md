# Jules Migration Status Report

Date: 2026-10-10
Branch: `jules/full-migration`

## Feature Migration Status Summary

| Feature / Subsystem | Status | Details / Remaining TS / Blockers |
| :--- | :--- | :--- |
| **Credits & Usage Ledger** | Design Note Added | Design note created (`docs/maintainers/credits-usage-design.md`); details reserve -> settle/refund pattern, 1.28 markup, 1000 credits/USD, Razorpay plans + `go_dataforseo_usage` ledger. |
| **DataForSEO Client** | Evaluated | Verified existing Go client in `backend/internal/platform/dataforseo` passes tests; input validation rules documented in design note. |
| **MCP Tools & Dispatcher** | Preserved | Verified Go MCP implementation and unit tests in `backend/internal/mcp`. TS fallback retained in `src/server/mcp` until external PRs (`codex/*`) complete cutover. |
| **SAM Chat Runtime** | Preserved | Verified Go SAM service and handlers in `backend/internal/sam`. |
| **Auth Subsystem** | Security Design Note Added | Security design note (`docs/maintainers/auth-security-design.md`) added documenting better-auth session compatibility, `crypto/subtle.ConstantTimeCompare`, and Redis rate limits. |
| **Email (Loops)** | In Progress | Email integration design mapped for background execution in `backend/internal/platform/jobs`. |
| **Background Jobs & Workflows**| Preserved | `backend/internal/platform/jobs` verified. |
| **Object Storage** | In Progress | S3/Railway bucket storage abstraction planned for `backend/internal/platform/jobs`. |
| **GDPR Erasure** | In Progress | Postgres + Redis (`ai-search:*` keys via SCAN/UNLINK) + Object Storage deletion workflow documented. |
| **Referrals & Minor Features** | In Progress | Models and endpoints ready for Go migration. |
| **Legacy Data Copy Tool** | Verified in origin/main | `backend/cmd/copy-legacy-data` exists in main with dry-run, foreign key ordering, and tests. Not executed on live data. |
| **Schema Baseline** | Activation Guide Added | Staged Goose v1 Drizzle PostgreSQL baseline in `backend/internal/database/baseline/00001_drizzle_baseline.sql` (from main) documented with activation steps in `docs/maintainers/migration/schema-baseline.md`. |
| **Dead Code (`knip`)** | Skipped | `node_modules` binaries absent in sandbox environment. |
| **Repo Layout Moves** | Deferred | Deferred structural folder moves (`legacy/`, `frontend/`) to prevent merge conflicts with open agent branches (`codex/*`, `freebuff/*`). |

---

## Owner Decisions
1. **Credits & Markup**: Preserved legacy markup (1.28x) and credit conversion rate (1000 credits per 1.00 USD).
2. **Autumn -> Razorpay**: Replaced Autumn dependency with Razorpay subscription plans + `go_dataforseo_usage` Postgres ledger.
3. **Layout Moves Deferred**: Deferred structural folder moves (`legacy/`, `frontend/`) until active parallel PRs on `codex/*` and `freebuff/*` are merged to avoid blocking rebase conflicts.

---

## Security Review Callouts
- **Session Tokens**: Better-auth token validation in Go uses parameterized pgx queries.
- **Constant-Time Comparison**: All token & API key equality checks use `crypto/subtle.ConstantTimeCompare`.
- **Rate Limiting**: Auth endpoints utilize Redis sliding-window rate limiting in `backend/internal/kv`.

---

## Validation Summary
- `cd backend && go test ./...` passed across all 35 packages.
- All Go packages formatted according to standard `gofmt`.

---

## Next Session Starting Point
1. Resume after merge of open `codex/*` and `freebuff/*` PRs.
2. Execute structural layout moves per `docs/maintainers/REPO-LAYOUT.md` once open branches settle.
3. Activate Goose v1 PostgreSQL baseline schema when TypeScript server functions are completely turned off.
