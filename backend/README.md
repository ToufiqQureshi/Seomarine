# Seomarine backend (Go)

## Run

```sh
DATABASE_URL=postgres://user:pass@localhost:5432/seomarine \
REDIS_URL=redis://localhost:6379/0 \
BETTER_AUTH_SECRET=<the legacy app's secret> \
UPSTREAM_APP_URL=http://localhost:3000 \
PUBLIC_URL=http://localhost:8080 \
PORT=8080 go run ./cmd/server
```

| Variable                  | Required | Meaning                                                                                                         |
| ------------------------- | -------- | --------------------------------------------------------------------------------------------------------------- |
| `DATABASE_URL`            | yes      | Postgres shared with the legacy app.                                                                            |
| `REDIS_URL`               | yes      | Redis, e.g. `redis://localhost:6379/0` or `rediss://` for TLS.                                                  |
| `BETTER_AUTH_SECRET`      | yes      | The legacy app's secret (at least 32 characters). It verifies the signed `better-auth.session_token` cookie.    |
| `UPSTREAM_APP_URL`        | yes      | Absolute `http(s)` URL of the legacy TypeScript app. Every route the Go server does not own is proxied there.   |
| `PUBLIC_URL`              | yes      | The site's public origin, e.g. `https://seomarine.com`, no path. Canonical, Open Graph and JSON-LD URLs use it. |
| `PORT`                    | no       | Listen port, default `8080`.                                                                                    |
| `TRUSTED_PROXY_CIDRS`     | no       | Comma-separated CIDRs of reverse proxies that overwrite `CF-Connecting-IP` and append `X-Forwarded-For`. If unset, forwarded IP headers are ignored. Never include untrusted client ranges. |
| `RAZORPAY_KEY_ID`         | no       | Razorpay API key id. Set all four `RAZORPAY_*` variables or none; without them billing endpoints answer `503`.  |
| `RAZORPAY_KEY_SECRET`     | no       | Razorpay API key secret.                                                                                        |
| `RAZORPAY_WEBHOOK_SECRET` | no       | Secret of the Razorpay webhook pointed at `/webhooks/razorpay`.                                                 |
| `RAZORPAY_PLAN_ID_PRO`    | no       | Razorpay plan id (`plan_...`) of the monthly Pro plan.                                                          |

At startup the server connects to Postgres, applies pending migrations,
connects to Redis, and fails if any step fails. It shuts down gracefully on
SIGINT or SIGTERM.

## Routes

- `GET /healthz`: liveness. The process is up.
- `GET /readyz`: readiness. Postgres and Redis answer within 2 seconds,
  otherwise 503.
- `GET /`: the landing page for visitors without a valid session; signed-in
  users get the app (proxied). `GET /pricing`: the pricing page, for
  everyone. Both are rendered once at startup from `internal/site`
  (`html/template`, no JavaScript) and send `ETag`, a strict CSP and
  `Vary: Cookie` on `/`. Prices are the `FreePriceINR`/`ProPriceINR`
  constants in `internal/site/site.go`; keep `ProPriceINR` equal to the
  Razorpay plan's amount.
- `GET /site/...`: the pages' embedded assets (logo, font, social card).
  The pages link them with a `?v=<content hash>`, which is cached for a
  year; other requests are revalidated by `ETag`.
- `GET /t.js`: the analytics tracker (under 1.5 KB, no cookies, no
  dependencies). Sites add the snippet from
  `POST /api/v1/projects/{projectId}/analytics/site`. It reports a pageview
  on load and on SPA `history.pushState`/`popstate`, uses `sendBeacon`, and
  does nothing when Do Not Track or Global Privacy Control is on. Cached for
  an hour, then revalidated by ETag.
- `POST /collect`: tracker events, `{"k": siteKey, "u": url, "r": referrer,
"w": screenWidth}`, answered with `202`. Open CORS, 4 KB body limit,
  120 events a minute per IP and site (`429` above that). The page host must
  be the project's domain or a subdomain of it when the project has one
  (`403` otherwise). Crawlers and headless browsers are accepted and dropped.
- `POST /api/v1/projects/{projectId}/analytics/site`: the project's site key
  and tracker snippet, created on first call.
- `GET /api/v1/projects/{projectId}/analytics/summary?from=YYYY-MM-DD&to=YYYY-MM-DD`:
  visitors, pageviews, a daily series and the top pages, channels, AI
  sources, referrers and devices for whole UTC days, at most 366 of them.
- `GET /api/v1/billing/status`: the plan of the session's active workspace,
  `{"plan": "free"|"pro", "status": "<Razorpay status>"|"none",
"currentPeriodEnd": RFC 3339|null}`. Any member may read it.
- `POST /api/v1/billing/checkout`: starts a Pro subscription and returns
  `{"subscriptionId", "keyId"}` for Razorpay Checkout. Owners and admins
  only (`403` otherwise); `409` when the workspace already has a live
  subscription; `502` when Razorpay fails.
- `POST /webhooks/razorpay`: Razorpay subscription webhooks, verified by
  `X-Razorpay-Signature` (`401` when it is missing or wrong).
- `/api/v1/...`: needs a valid legacy session cookie, otherwise 401.
  `/api/v1/projects/{projectId}/...` also needs membership of the project's
  organization; any other project answers 404. Errors are JSON:
  `{"error": {"code": "...", "message": "..."}}`.
- Everything else is reverse-proxied to `UPSTREAM_APP_URL` with the original
  `Host`, cookies, path and query.

## Analytics

**Countries.** The server embeds DB-IP Country Lite (October 2026, CC BY 4.0;
attribution in `internal/analytics/geo/ATTRIBUTION.md`). The IP is used only
in memory for lookup and the existing rotating visitor hash. Events store an
ISO country code or NULL, never the IP. The Lite data has reduced accuracy
and must be refreshed monthly. `GET /api/v1/analytics/{siteId}/countries`
accepts `from`, `to`, `page` (default 1), and `limit` (default 10, maximum
100). It returns country rows with visitor counts and percentages of all
visitors, including those whose country could not be found. The `siteId` is
returned by the analytics site endpoint.

**Privacy.** No cookies and no stored IP. A visitor is
`SHA-256(daily salt, site key, IP, user agent)`. The salt is random per UTC
day, lives only in Redis and expires after 48 hours, so a visitor cannot be
followed across days and old hashes cannot be recomputed. Visitors over a
range are therefore the sum of each day's unique visitors. Only the path is
stored, never the query string.

**Channels.** Each pageview is attributed in this order:

1. A referrer on the page's own host, or on the project's domain or a
   subdomain, is internal navigation. It counts as a pageview but not as a
   channel visit, and its `utm_source` is ignored.
2. A referrer on an AI assistant (`chatgpt.com`, `chat.openai.com`,
   `perplexity.ai`, `gemini.google.com`, `bard.google.com`, `claude.ai`,
   `copilot.microsoft.com`, and others such as DeepSeek, Mistral, Meta AI,
   Grok and Poe as `other_ai`) is `ai`.
3. A `utm_source` naming an assistant (`chatgpt.com`, which ChatGPT appends
   to links it cites, or `perplexity`, `claude`, ...) is `ai`. Most
   assistants hide the referrer, so this recovers AI traffic that would
   otherwise read as direct.
4. A referrer on a search engine (`google.*`, `bing`, `duckduckgo`, ...) or
   social network (`facebook`, `t.co`, `linkedin`, ...) is `search` or
   `social`.
5. Any other referrer is `referral`.
6. Without a referrer, a `utm_source` naming a search engine or social
   network is that channel.
7. Everything else is `direct`.

Hosts compare case-insensitively without `www.`. The tables are in
`internal/analytics/classify.go`.

## Billing

One Pro plan, sold as a Razorpay subscription per workspace (organization).
Checkout creates the subscription with the workspace id in its notes; the
browser then pays for it with Razorpay Checkout. Razorpay reports what
happens next through webhooks, so subscribe the webhook to
`subscription.activated`, `.charged`, `.cancelled`, `.halted` and
`.completed`; other events are acknowledged and ignored.

- Each event carries the subscription's full state, which replaces the
  stored state unless the stored one is newer, so redeliveries and
  out-of-order deliveries are safe. An event id (`X-Razorpay-Event-Id`) is
  applied once.
- The workspace is on Pro while its subscription is `authenticated`,
  `active` or `pending` (Razorpay retrying a failed charge). `halted`,
  `cancelled`, `completed` and the rest are the free plan.
- A checkout never replaces a subscription that could still charge
  (`authenticated`, `active`, `pending`, `halted`, `paused`). An unpaid one
  is replaced by a fresh subscription.

## Database

Migrations live in `internal/database/migrations` (goose, embedded SQL) and
run at startup under a Postgres advisory lock. The Go server shares the
legacy app's database, so every Go-owned table, including the
`go_schema_migrations` history, is prefixed `go_`.

## Checks (all mandatory, see /CLAUDE.md)

```sh
go mod tidy && gofmt -l . && go vet ./... && staticcheck ./... \
  && golangci-lint run ./... && go test -race -count=1 -cover ./... \
  && govulncheck ./... && deadcode ./...
```

Integration tests need `TEST_DATABASE_URL` (Postgres) and `TEST_REDIS_URL`
(Redis). CI always sets them, and the tests fail there if one is missing.
