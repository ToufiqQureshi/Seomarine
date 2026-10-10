# Migration work report

Date: 2026-10-10. Base: `origin/main` at `c2e31b0`. PR #36 was already
merged by the owner and was excluded from this work. The local main checkout
was synced before the worktrees were created. No production migration or data
copy was run.

## Draft pull requests

Merge these in dependency order. Each change was made on its own `codex/*`
branch and worktree.

| Order | Work | Draft PR | Status |
| --- | --- | --- | --- |
| 1 | MCP Go dispatcher with TypeScript fallback (A1) | [#39](https://github.com/ToufiqQureshi/Seomarine/pull/39) | Ready for review |
| 2 | Selected read/free MCP tools in Go (A2) | [#40](https://github.com/ToufiqQureshi/Seomarine/pull/40) | Partial scope; see below |
| 3 | Goose baseline for the Drizzle PostgreSQL schema (K) | [#41](https://github.com/ToufiqQureshi/Seomarine/pull/41) | Ready for review; needs database validation |
| 4 | Idempotent, dry-run legacy D1-to-Postgres copy tool (J) | [#42](https://github.com/ToufiqQureshi/Seomarine/pull/42) | Ready for review; tool not run |

## Task status

| Task | Result and next work |
| --- | --- |
| A1 | Go dispatcher handles MCP paths and proxies unported tools to TypeScript. PR #39. |
| A2 | PR #40 ports SERP location search, site-audit list/status/pages/delete, and saved-keyword list/save/remove. GSC, GA4, reports/templates/sharing, rank-tracker reads, and remaining audit output still use the TypeScript fallback. Finish these before calling A2 complete. |
| A3 | Billed DataForSEO MCP tools remain on TypeScript. The Go path needs a credit reservation/settlement design so a failed or retried billed request cannot charge incorrectly. |
| A4 | ChatGPT app flow and MCP contract coverage remain. Do not remove `src/server/mcp` until every tool and its contract is in Go. |
| B | Go currently has the SAM session registry. Agent loop, transcripts, tools, skills, metering, and streaming remain in the TypeScript runtime. Credit metering and the job/runtime design need to precede a safe cutover. |
| C | Existing Go projects/project-context/dashboard pieces remain partial. Reports, activation, referrals, and their callers still need a full port. |
| D | Complete Postgres, Redis, and object-storage erasure job remains. Object-storage replacement in G is a dependency. |
| E | Loops transactional and lifecycle mail integration remains. |
| F | `backend/internal/platform/jobs` exists; Cloudflare Workflows and cron are not fully moved to it. |
| G | R2-to-Railway-bucket/S3 storage migration remains. Storage config and a data migration plan are needed. |
| H | Go reads existing Better Auth sessions. Login, signup, password reset, OAuth, organization, and member management still need Go APIs and a cutover plan. |
| I | Razorpay plans and `go_dataforseo_usage` exist. Autumn credit reservation, balance, and settlement behavior remain; billed Go tools depend on this. |
| J | PR #42 adds a default dry-run copier, explicit `--apply`, schema checks, foreign-key ordering, and conflict-safe inserts. The copier was not run. Source writes must be frozen during an actual copy. |
| K | PR #41 captures the Drizzle PostgreSQL tables in Goose version 1 while preserving the version number for databases that already applied it. Apply and rollback have not been exercised against a database. |
| L | `pnpm knip --reporter compact` exited successfully with no dead TypeScript candidates; no deletion PR was justified. Rerun after further feature ports. |
| M | Layout moves remain. The `go.mod`/`db/` move requires coordinated build paths, including Dockerfile/deploy files owned by Freebuff. The `legacy/` and `frontend/` moves require the remaining TypeScript callers and page switches to finish. Keep each eventual move mechanical and in a separate PR. |

## Validation and boundaries

- PR #40: `go test ./internal/mcp ./cmd/server` and `go vet ./internal/mcp ./cmd/server` passed.
- PR #41: SQL markers and version placement were checked. Goose CLI validation was unavailable because optional CLI dependencies were absent from `go.sum`; no database was migrated.
- PR #42: `pnpm exec tsc --noEmit`, three focused Vitest cases, and `git diff --check` passed. The copy command was not executed.
- React page switches, Dockerfile/deploy, observability, and API contract check files were left to Freebuff as directed.
- The untracked `backend/internal/dashboard/` directory in the main checkout was left untouched.

The requested A–M migration is **not complete**. In particular, PR #40 covers
only part of A2, and tasks A3–I and M still need implementation PRs. Do not
merge or delete the TypeScript backend based on this report alone.
