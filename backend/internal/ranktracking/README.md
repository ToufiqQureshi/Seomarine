# Rank tracking

## Legacy behavior and edge cases found before porting

- Configs are keyed by project, normalized domain, country and optional canonical city. An archived config reactivates with its keywords/history; an active duplicate fails. At most 500 active configs per project. A local name is probed in the DataForSEO sandbox before saving.
- Devices are desktop/mobile/both; SERP depth is 10–100 in steps of 10. SERP allows any supported language in any country; keyword metrics use a country-served language instead. Keywords cap at 1,000 per config, 200 UTF-16 units each; match-case keywords can coexist with lowercase twins.
- Schedule modes are manual/daily/weekly/monthly. Chosen hour, minute, weekday and IANA timezone set the initial UTC anchor; later runs advance that anchor without drift. Month-end checks preserve the user's last local day. Unchanged settings do not move an existing anchor.
- Creating a config or adding keywords may auto-check; an imminent scheduled run within one hour takes precedence. No-keyword configs do not run. Hosted free plans cannot run checks or metric refreshes; self-hosted mode is open.
- Manual checks use live SERP; scheduled checks use cheaper queued tasks with at most 100 tasks per post. Cost estimates account for every keyword/device pair, depth, provider-call rounding and a fivefold charge for advanced search operators. A requested maximum credit ceiling must be checked again against the final keyword list before provider work.
- At most one active run per config. A stale run may be failed before replacement; polling does not mutate it. Due config claims are conditional on the observed schedule token, advance eagerly to avoid retry storms, and obey task-unit and wall-clock budgets.
- The workflow checks that the config remains active, prepares keywords, checks credit balance, writes snapshots incrementally, and finalizes based on actual distinct keyword snapshots. Zero snapshots mean failure; partial completion keeps the first provider error. A failed run never advances `lastCheckedAt`.
- A Go retry must not double-bill DataForSEO or duplicate snapshots. Job claiming uses `FOR UPDATE SKIP LOCKED`; provider task IDs and persisted per-pair states must survive a crash. Canceled jobs release work safely; scheduled timing uses a controlled clock in tests.
- Results include desktop/mobile positions, previous positions, ranking URLs, SERP features, run status, keyword history, trend buckets and a position matrix. A newer failed run must not hide the freshness of older snapshots. Every read/write is scoped through project and organization.

## Migration status

This package is an inventory for the Go port. The legacy TypeScript API and Cloudflare workflow remain in service until the Go jobs, tables, HTTP contracts and React callers are verified together.
