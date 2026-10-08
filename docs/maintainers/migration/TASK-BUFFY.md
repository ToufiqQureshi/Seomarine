# TASK: Buffy

Read first: `FEATURES.md` (every feature that must survive; mark your rows DONE with the Go endpoint), `STATUS.md` (your rows = Owner "Buffy"), `parallel-tasks.md` (COMMON rules),
`codex-mega-task.md` (safety rules only; ignore its item list).

## Your own worktree (do not work in the main checkout)

```
cd D:\seomarine\Seomarine
git fetch origin
git worktree add ..\wt-buffy -b mig/buffy-base origin/main
cd ..\wt-buffy
```

All your work happens in `D:\seomarine\wt-buffy`. Codex works in `D:\seomarine\wt-codex`.
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

## You only need `platform/httpx` and `platform/pgdb` (already on main)

`platform/jobs`, `dataforseo`, `entitlements` are being built by Codex. If your feature
needs one before it lands, write a small private helper inside your feature package and
note it in `docs/maintainers/migration/<feature>.md`. Do NOT edit `platform/*`.

## Your order (Wave 1)

1. `audit` (6.8k lines, biggest; includes lighthouse, crawlerAccess; crawler needs an
   SSRF guard and robots.txt handling).
2. `billing` (replace the remaining TS; keep Razorpay webhook behavior identical).
3. `ga4`, `gsc` (incl. searchPerformance), `google` (OAuth, encrypted tokens, refresh).
4. `backlinks` (+ ahrefs adapter if still used), `ai-search`, `sam` (+ samAccess).
5. `reports` (+ templates), `branding`.
6. `dashboard`, `activation`, `referrals`, `gdpr`, `email` (behind a `Mailer` interface).
7. Docs: rewrite `CLAUDE.md`, `AGENTS.md`, `README.md`, `ROADMAP.md`, `docs/*` so they
   describe the Go backend that exists on `main`. Update the docs each PR made wrong as
   you go; do the full rewrite last.

## Never

Edit Codex rows. Skip, weaken or delete a test to get green. Change legacy table schemas.
Touch `LICENSE-OPENSEO-MIT`. Commit secrets.
