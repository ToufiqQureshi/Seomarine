# Rank tracking

## Legacy behavior and edge cases found before porting

- Configs are keyed by project, normalized domain, country and optional canonical city. An archived config reactivates with its keywords/history; an active duplicate fails. At most 500 active configs per project. A city name must exist in the country's location registry (same list as the picker). Deliberate difference from legacy: the legacy sandbox probe failed open, this check fails closed (503) when the registry is unreadable.
- Devices are desktop/mobile/both; SERP depth is 10–100 in steps of 10. SERP allows any supported language in any country; keyword metrics use a country-served language instead. Keywords cap at 1,000 per config, 200 UTF-16 units each; match-case keywords can coexist with lowercase twins.
- Schedule modes are manual/daily/weekly/monthly. Chosen hour, minute, weekday and IANA timezone set the initial UTC anchor; later runs advance that anchor without drift. Month-end checks preserve the user's last local day. Unchanged settings do not move an existing anchor.
- Creating a config or adding keywords may auto-check; an imminent scheduled run within one hour takes precedence. No-keyword configs do not run. Hosted free plans cannot run checks or metric refreshes; self-hosted mode is open.
- Manual checks use live SERP; scheduled checks use cheaper queued tasks with at most 100 tasks per post. Cost estimates account for every keyword/device pair, depth, provider-call rounding and a fivefold charge for advanced search operators. A requested maximum credit ceiling must be checked again against the final keyword list before provider work.
- At most one active run per config. A stale run may be failed before replacement; polling does not mutate it. Due config claims are conditional on the observed schedule token, advance eagerly to avoid retry storms, and obey task-unit and wall-clock budgets.
- The workflow checks that the config remains active, prepares keywords, checks credit balance, writes snapshots incrementally, and finalizes based on actual distinct keyword snapshots. Zero snapshots mean failure; partial completion keeps the first provider error. A failed run never advances `lastCheckedAt`.
- A Go retry must not double-bill DataForSEO or duplicate snapshots. Job claiming uses `FOR UPDATE SKIP LOCKED`; provider task IDs and persisted per-pair states must survive a crash. Canceled jobs release work safely; scheduled timing uses a controlled clock in tests.
- Results include desktop/mobile positions, previous positions, ranking URLs, SERP features, run status, keyword history, trend buckets and a position matrix. A newer failed run must not hide the freshness of older snapshots. Every read/write is scoped through project and organization.

## Migration status

Done (Go, tested against a real Postgres):

- Rules: `schedule.go` (next-check anchors), `cost.go` (live vs queued estimates), `keywords.go` (domain and keyword normalising, caps).
- Storage: migration `00009`, `store.go` (pgx, no ORM). The keyword cap is enforced inside one transaction that locks the config row, so racing adds cannot pass 1,000.
- Service and API: configs (list, create, update, archive, reactivate) and keywords (list, add, remove, estimate cost). Contract in `backend/api/ranktracking.yaml`; every route is a POST with a JSON body, like the audit API.

- Results: latest table with compared positions, latest run, keyword history, trend buckets and the position matrix, read from completed runs only. The newest snapshot of a keyword and device wins, subset runs included; trend and matrix use full runs only. Comparison is the newest snapshot at or before the period mark, else the keyword's first snapshot. Freshness comes from the newest completed snapshot, so a newer failed run does not hide it.

- Manual checks: `checks/start` creates a pending run and queues a job (queue `rank_check`, the run id is the idempotency key). The worker (`Checks.Execute`) looks up every keyword and device live on DataForSEO, in batches of 10 keywords, and stores one snapshot per pair. A pair that already has a snapshot is never asked again, so a retry after a crash does not bill twice. The first provider error is kept on the run; billing and invalid-field errors stop the run, because every further task would fail and still be billed. A run with no snapshot fails; a partial run completes with "Checked X of Y keyword(s)". A completed run stamps `last_checked_at`; a failed one does not.
- One active run per config comes from the partial unique index. A blocking run is cleared when its job ended without finishing it, or has no job a minute after it started (the Go replacement for the legacy workflow-state check).
- A paid plan is required when billing is configured; without billing the check runs ungated, as on a self-hosted install.

- Scheduled checks: `Ticker.Tick` runs every five minutes on every instance. It lists due configs oldest first (joining the legacy `projects` table for the organization), claims each with a compare-and-set on `next_check_at`, then starts a run. The anchor moves before the run starts, so a failing start cannot make a config due again and bill twice; it moves by whole intervals from the due time, so a late tick does not drift the schedule. A plan error leaves the anchor alone (the retry), a missing plan or no keywords advances it with `plan_required` or `no_keywords`, a blocking run gives the slot back. Work per tick is capped at 1,000 tasks (the first start is always admitted) and 3 minutes.
- Scheduled runs use the provider's task queue (about 30% of the live price): post up to 100 tasks per request, store every task id at once in `go_rank_check_tasks`, poll `task_get` (free, so it goes through `DoUnmetered` and is never recorded as spend) at 4, 6, 8, 10, 12 and 15 minutes, and look up rejected, failed or timed-out tasks live. After a crash the retry collects the stored tasks instead of posting, and paying, again.
- Keyword metric refresh is `POST /api/v1/projects/{projectId}/rank-tracking/keywords/metrics/refresh`. It requires the tracked config, reuses the keyword research provider's bounded batches and country-served language rules, and sends lowercased distinct terms so case variants do not duplicate billed work. City configs use city-scoped Ads volume/CPC and national Labs difficulty. Hosted deployments require a paid plan before any provider call; self-hosted deployments remain ungated. Metric writes are one config-scoped statement and update only rows still present.

Not ported yet: per-run credit holds, the React switch and the MCP tools. Until the switch nothing in the UI calls these routes, so the legacy TypeScript API and Cloudflare workflow stay in service.

## Decisions for the owner

- Credit maths (`costMarkup` 1.28, 1000 credits per USD) is copied from the legacy app. Seomarine's Razorpay plan model may want different numbers.
- `go_rank_*` start empty. Existing customer data needs a one-time copy from the legacy tables before the switch.
- Scheduled checks: a task that was posted but whose id was not stored before a crash (the window is one database write) would be posted again. Posting twice is billed twice, but the window is tiny and the alternative, a live fallback for every scheduled pair, costs more.
- Credits: the legacy app held and settled Autumn credits around each check. Here the spend control is the paid-plan gate, the optional `maxCostCredits` ceiling and the usage ledger (`go_dataforseo_usage`). A real balance check waits for the owner's credit model.
- Deliberate differences from the legacy app: a tie on `checked_at` picks the later-written snapshot (the legacy join could return both); a keyword over 200 UTF-16 units is returned as rejected, not silently dropped; domains must be plain ASCII hostnames (no ports, no unicode, labels up to 63 characters), because a name the provider rejects is still billed; the MCP-only credit-ceiling approval flow is not ported (it returns with the MCP tools).
- City-level configs (`locationName`) are checked against the registry; without a provider key they answer 503.
- The project cap (500 active configs) is checked before insert, not locked, so two simultaneous creates could end one over.
