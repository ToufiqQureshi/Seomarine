# TASK: Codex

Read first: `STATUS.md` (your rows = Owner "Codex"), `parallel-tasks.md` (COMMON rules),
`codex-mega-task.md` (safety rules; its item list is replaced by the order below).

## Your own worktree (do not work in the main checkout)

```
cd D:\seomarine\Seomarine
git fetch origin
git worktree add ..\wt-codex -b mig/codex-base origin/main
cd ..\wt-codex
```

All your work happens in `D:\seomarine\wt-codex`. Buffy works in `D:\seomarine\wt-buffy`.
Never edit files in the other worktree. Never switch the main checkout's branch.

## Loop for every item

```
git fetch origin
git switch -c mig/<feature> origin/main     # new branch per item
# 1 Go code + parity tests, 2 frontend switch, 3 delete TS, 4 mark rows DONE in STATUS.md
$env:GOTMPDIR='D:\gotmp'                     # Windows blocks test exes in %TEMP%
gofmt, go vet, go test ./..., pnpm tsc --noEmit, pnpm exec prettier --write <changed>, pnpm knip
git fetch origin; git rebase origin/main     # resolve conflicts, rerun checks
git push origin HEAD:main                    # only if all checks are green
```

If the push is rejected (someone pushed first), rebase again and retry. If CI on `main`
turns red because of your commit, fix it immediately before the next item.

## Your order (Wave 1, biggest value first)

1. `platform-lib` leftovers + fixes on `platform/httpx`: remove the `auth` import,
   add panic-recovery middleware, log the response status, `Mount(mux, Deps)` signature,
   unexport handlers not needed outside (see review notes in STATUS history).
2. `platform/jobs`, `platform/dataforseo`, `platform/entitlements` (allow-all stub),
   OpenAPI codegen.
3. `identity` (organizations, workspace, onboarding, config, auth). A draft is on local
   branch `mig/l1-identity-m1` (commit `b0bc53d`): rebase it, do not rewrite it.
4. `projects`, `project-context`, `domain`, `keywords` (incl. serp-locations).
5. `rank-tracking` (+ its workflows), `workflows` leftovers.

Wave 2 (only after every Wave 1 row, yours and Buffy's, is DONE): `mcp`, `db-schema`,
Cloudflare/Alchemy/wrangler removal, Go serves the React build.

## Never

Edit Buffy rows. Skip, weaken or delete a test to get green. Change legacy table schemas.
Touch `LICENSE-OPENSEO-MIT`. Commit secrets.
