# Repository cleanup inventory

Branch: `chore/cleanup-useless-files`, based on `origin/main` at
`700e408608c41a3a2bbad69fb84f4c7cf156bcaf`.

This is an inventory only. No repository files were deleted. Rows marked
`KEEP` include the proof that prevents safe deletion under this task's rules.

## Step 1: inventory and Step 2: deletion proof

Sizes for files use bytes; directory sizes are recursive totals. `.env.local`
was not opened, and no secret values were read or copied into this report.

| Path                                                                                                     |                           Size | Why it looked potentially useless                | Proof / references                                                                                                                                                                                                                          | Action            |
| -------------------------------------------------------------------------------------------------------- | -----------------------------: | ------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------- |
| `.env.local` (ignored)                                                                                   |                        2,661 B | Local-only environment file                      | Referenced by local-development, migration, and self-hosting docs/scripts. It may contain secrets; contents were not inspected.                                                                                                             | KEEP              |
| `.wrangler/` (ignored)                                                                                   |           26 files / 365,319 B | Generated Wrangler state/cache                   | `.gitignore:8` ignores it; `deploy/docker/Dockerfile.dockerignore:5` excludes it from Docker context. Wrangler is an active configured platform.                                                                                            | KEEP              |
| `dist/` (ignored)                                                                                        |       384 files / 24,889,750 B | Generated build output                           | `package.json:16` deploys generated `dist/seomarine_audit/wrangler.json`; `deploy/alchemy/alchemy.run.ts:372,431,434` consumes built artifacts.                                                                                             | KEEP              |
| `node_modules/` (ignored)                                                                                | 89,455 files / 1,451,765,566 B | Installed dependencies                           | Explicitly protected by the task; required by the requested pnpm checks.                                                                                                                                                                    | KEEP              |
| `.tanstack/`                                                                                             |                  0 files / 0 B | Empty generated directory                        | No files or content to remove; `.gitignore` identifies it as generated TanStack state.                                                                                                                                                      | KEEP              |
| `.output/`, `coverage/`, `*.log`, `*.tsbuildinfo`                                                        |               0 matching files | Common generated outputs                         | Ignored-file scan and targeted filename scan found no matching repository files.                                                                                                                                                            | KEEP (none found) |
| `*.bak`, `*.orig`, `*.tmp`, `*~`                                                                         |               0 matching files | Temporary/backup files                           | Targeted filename scan found no matching files.                                                                                                                                                                                             | KEEP (none found) |
| `.DS_Store`, `Thumbs.db`, root archives/screenshots                                                      |               0 matching files | OS/editor or stray binary files                  | Targeted filename scan and root listing found none. The tracked `src/public/` images/icons are product assets, not root junk.                                                                                                               | KEEP (none found) |
| Tracked zero-byte files                                                                                  |                        0 files | Empty-file candidates                            | `git ls-files` checked 1,425 tracked paths; zero were empty.                                                                                                                                                                                | KEEP (none found) |
| `backend/internal/site/static/social-card.png` and `src/public/social-card.png`                          | Exact duplicate; 48,320 B each | Duplicate image                                  | SHA-256 comparison matched. Backend use: `backend/internal/site/site.go:126`; frontend route references `/social-card.png` in `src/routes/s/$token/og[.]png.ts:31`. `backend/` is explicitly protected.                                     | KEEP              |
| `backend/internal/site/static/logo.svg` and `src/public/logo.svg`                                        |    Exact duplicate; 733 B each | Duplicate image                                  | SHA-256 comparison matched. Backend templates/tests use the embedded asset; frontend components, route metadata, and `src/public/site.webmanifest` reference `/logo.svg`. `backend/` is explicitly protected.                               | KEEP              |
| `LICENSE`, `LICENSE-OPENSEO-MIT`, `CLAUDE.md`, `AGENTS.md`, `ROADMAP.md`                                 |                Protected files | Never-delete files                               | Explicitly protected by the task. OpenSEO mentions outside the protected license attribution are limited to brand/legal guidance in `AGENTS.md`, `CLAUDE.md`, and the required attribution in `LICENSE`.                                    | KEEP              |
| `wrangler.jsonc`, `wrangler.audit.jsonc`, `deploy/alchemy/*`, Cloudflare/Alchemy config and dependencies |                        Tracked | Possible removed-platform leftovers              | Active package scripts use Wrangler and Alchemy (`package.json:16-35`); self-hosting and preview docs and deploy files reference these configs. The migration task reserves platform removal for item 40.                                   | KEEP              |
| `tests/badseo/wrangler.jsonc` and its Cloudflare test fixture                                            |                        Tracked | Possible stale deploy config                     | `tests/badseo` is included in `pnpm ci:check` through its TypeScript config and the fixture has its own documented deployment/test setup.                                                                                                   | KEEP              |
| All other tracked files                                                                                  |      1,425 tracked files total | Potential unused files, exports, or dependencies | `pnpm knip` exited 0 with no unused file/export/dependency diagnostics. `scripts/`, `deploy/`, `specs/`, and `tests/` were cross-checked against package scripts, CI workflows, docs, and imports; no safely deletable row was established. | KEEP              |
| `docs/maintainers/migration-progress.md`                                                                 |                        Tracked | Migration status may be stale                    | It is migration-status documentation. Do not alter the 40-item migration work as requested; no deletion is justified.                                                                                                                       | KEEP              |
| Other tracked docs                                                                                       |                        Tracked | Possible stale descriptions                      | Cloudflare, Docker, Postgres, and migration docs map to active configs/scripts. No specific document was proven to describe a removed component.                                                                                            | KEEP              |

## Step 2 reference-scan scope

The potential ignored-output and duplicate candidates were searched for path
references/imports across `src/`, `backend/`, `scripts/`, `tests/`, `deploy/`,
`docs/`, `package.json`, and `.github/`. Exact matches and their live uses are
listed above. `node_modules/`, `.env.local`, `backend/`, `drizzle/`,
`src/client/`, `src/server/`, `src/serverFunctions/`, `src/db/`, lockfiles,
and `.github/` were not modified.

## Local branches and worktrees (list only)

Merged local branches: `chore/consistent-lf`, `feat/2-2b-analytics-countries`,
`feat/2-4-ai-visibility`, `feat/5-3-live-usage-cancel`, `main`, and this
cleanup branch (same starting commit as `main`).

Not-merged local branches: `backup/main-before-sync-20261008`,
`mig/l1-identity-m1`.

Worktrees at inventory time: `D:/seomarine/Seomarine` on this cleanup branch.
No other worktree was registered. No branch or worktree was deleted.

## Baseline checks (before report change)

| Check                                                   | Result                                                                                                                                                                                                     |
| ------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `pnpm install --frozen-lockfile`                        | PASS; lockfile already current. pnpm reported ignored dependency build scripts.                                                                                                                            |
| `pnpm ci:check`                                         | FAIL at `prettier --check .`; 1,129 existing files reported formatting differences. Knip/TypeScript/Oxlint stages did not run because the command stops at Prettier.                                       |
| `pnpm test`                                             | FAIL: 192/197 files passed; 5 files failed (4 tests failed, 1 suite failed before tests completed). Failures: Windows temp cleanup `EPERM`, child Node `ETIMEDOUT`, and a skill fixture frontmatter error. |
| `cd backend && go build ./...`                          | PASS.                                                                                                                                                                                                      |
| `cd backend && $env:GOTMPDIR='D:\gotmp'; go test ./...` | FAIL: Windows Application Control blocked `D:\gotmp\...\httpapi.test.exe`; other packages printed as passing.                                                                                              |

## After-report checks (before any deletion)

| Check                                                   | Result                                                                                                                                                                                                                  |
| ------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `pnpm install --frozen-lockfile`                        | PASS; lockfile already current. Same ignored dependency build-script warning.                                                                                                                                           |
| `pnpm ci:check`                                         | FAIL at `prettier --check .`; the same 1,129 existing files reported formatting differences. The new report passes an individual `prettier --check`. Later stages did not run.                                          |
| `pnpm test`                                             | FAIL: same result, 192/197 files passed; 5 files failed (4 tests failed, 1 suite failed before tests completed), with the same Windows cleanup `EPERM`, child Node `ETIMEDOUT`, and skill fixture frontmatter failures. |
| `cd backend && go build ./...`                          | PASS.                                                                                                                                                                                                                   |
| `cd backend && $env:GOTMPDIR='D:\gotmp'; go test ./...` | PASS; every package passed. This is better than baseline, where Windows Application Control blocked `httpapi.test.exe`.                                                                                                 |

## Deletion summary

No candidate passed the task's deletion/reference rules. Deleted files: none.
Size saved: **0 B**. No cleanup deletion was performed.

Branch pushed: `chore/cleanup-useless-files` at commit `5cdcc5f`.

Draft PR creation was attempted but GitHub returned `403 Resource not
accessible by integration`; no PR exists yet. Create/review link:
<https://github.com/ToufiqQureshi/Seomarine/compare/main...chore/cleanup-useless-files?expand=1>.
