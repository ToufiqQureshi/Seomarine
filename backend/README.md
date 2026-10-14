# Uplumo backend (Go)

## Run

```sh
DATABASE_URL=postgres://user:pass@localhost:5432/uplumo \
REDIS_URL=redis://localhost:6379/0 \
BETTER_AUTH_SECRET=<at least 32 random characters> \
STATIC_DIR=../frontend/dist \
PUBLIC_URL=http://localhost:8080 \
PORT=8080 go run ./cmd/server
```

| Variable | Required | Meaning |
| --- | --- | --- |
| `DATABASE_URL` | yes | Postgres. A new database is fully built by the migrations. |
| `REDIS_URL` | yes | Redis, e.g. `redis://localhost:6379/0` or `rediss://` for TLS. |
| `BETTER_AUTH_SECRET` | yes | At least 32 characters. Signs the `better-auth.session_token` cookie and email tokens, and by default derives the key that encrypts Google tokens. Never change it on a live site. |
| `PUBLIC_URL` | yes | The site's public origin, e.g. `https://uplumo.com`, no path. Canonical, Open Graph and JSON-LD URLs use it. |
| `BETTER_AUTH_URL` | no | Origin used in sign-in, verification and invitation links. Set it to the same value as `PUBLIC_URL`. |
| `STATIC_DIR` | no | Directory of the built React app (Vite `dist`). Every path the Go server does not own gets its `index.html`. Default `dist`. |
| `PORT` | no | Listen port, default `8080`. |
| `TRUSTED_PROXY_CIDRS` | no | Comma-separated CIDRs of reverse proxies that overwrite `CF-Connecting-IP` and append `X-Forwarded-For`. If unset, forwarded IP headers are ignored. Never include untrusted client ranges. |
| `RAZORPAY_KEY_ID`, `RAZORPAY_KEY_SECRET`, `RAZORPAY_WEBHOOK_SECRET` | no | Set all three, with at least one plan id below, or none. Without them billing status and credits remain read-only, and checkout, cancellation and webhooks answer `503`. The webhook secret belongs to the webhook pointed at `/webhooks/razorpay`. |
| `RAZORPAY_PLAN_ID_BASIC`, `RAZORPAY_PLAN_ID_PRO`, `RAZORPAY_PLAN_ID_AGENCY` | no | The monthly Razorpay plan (`plan_...`) that bills each plan. Its amount must equal the price in `internal/plans/catalog.go`. A plan without an id cannot be bought. |
| `PROVIDER_DAILY_LIMIT_USD` | no | Emergency limit, default 100, 1 to 10000. Once today's (UTC) DataForSEO and AI spend across every workspace reaches it, all paid lookups and AI answer `usage_paused` until midnight UTC. Each workspace also has its own daily limit (a third of its monthly credits), so set this above the sum of those; it is only for a bug. Monthly credits per plan are in `internal/plans/catalog.go`. |
| `STAFF_EMAILS` | no | Comma-separated addresses. A workspace owned by one of them, verified, gets every feature with no limits and pays nothing. For the owner and the team only. |
| `DATAFORSEO_API_KEY` | no | Base64 `login:password`. Without it keywords, rank tracking, backlinks, domain and AI search answer `503`. |
| `OPENROUTER_API_KEY`, `OPENROUTER_MODEL` | no | The model behind the assistant and AI writing. |
| `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` | no | Google sign-in, Search Console and Analytics. |
| `GOOGLE_TOKEN_ENCRYPTION_KEY` | no | Base64, 32 bytes. Encrypts Google tokens with its own key instead of the one derived from `BETTER_AUTH_SECRET`. |
| `LOOPS_API_KEY`, `LOOPS_TRANSACTIONAL_VERIFY_EMAIL_ID`, `LOOPS_TRANSACTIONAL_RESET_PASSWORD_ID`, `LOOPS_TRANSACTIONAL_INVITATION_ID` | no | Transactional email. The key, the verify ID and the reset ID are needed together. See `internal/email/README.md`. |
| `TURNSTILE_SECRET_KEY` | no | Bot check on sign-up. |
| `BYPASS_EMAIL_VERIFICATION` | no | `true` lets a local sign-up skip the email. Refused on an `https` site. |
| `SENTRY_DSN` | no | Sends error-level logs and panics to Sentry. See `internal/platform/errreport/README.md`. |
| `OPENAI_APPS_CHALLENGE` | no | Token served at `/.well-known/openai-apps-challenge`. |

On an `https` site the server also refuses to start when `DATAFORSEO_API_KEY`
or `OPENROUTER_API_KEY` is set without the `RAZORPAY_*` variables: without
billing nothing limits what a workspace can cost us.

At startup the server connects to Postgres, applies pending migrations,
connects to Redis, and fails if any step fails. It shuts down gracefully on
SIGINT or SIGTERM.

## Routes

This list covers the routes owned by the server itself. Each feature's routes
are in its own `internal/<feature>/README.md` and, where one exists,
`api/<feature>.yaml`.

- `GET /healthz`: liveness. The process is up. `GET /api/health` answers
  `{"status":"ok"}` and is what Railway checks.
- `GET /readyz`: readiness. Postgres and Redis answer within 2 seconds,
  otherwise 503.
- `GET /`: the landing page for visitors without a valid session; signed-in
  users get the app. `GET /pricing`, `/about`, `/contact`, `/terms`,
  `/privacy`, `/refunds` and `/shipping`: public pages, for everyone, plus
  `/robots.txt` and `/sitemap.xml`. All are rendered once at startup from
  `internal/site` (`html/template`, no JavaScript) and send `ETag`, a strict
  CSP, `no-transform` (so Cloudflare does not rewrite the support address)
  and `Vary: Cookie` on `/`. The operator name, address, phone and the
  "last updated" date of the legal pages are constants in the same file. Prices and the plan
  comparison come from `internal/plans/catalog.go`.
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
"w": screenWidth, "n": eventName}` (`n` only for a custom event such as a
  button click), answered with `202`. Open CORS, 4 KB body limit,
  120 events a minute per IP and site (`429` above that). The page host must
  be the project's domain or a subdomain of it when the project has one
  (`403` otherwise). Crawlers and headless browsers are accepted and dropped.
- `POST /api/v1/projects/{projectId}/analytics/site`: the project's site key
  and tracker snippet, created on first call.
- `GET /api/v1/projects/{projectId}/analytics/summary?from=YYYY-MM-DD&to=YYYY-MM-DD`:
  visitors, pageviews, a daily series and the top pages, channels, AI
  sources, referrers, devices and custom events (button clicks) for whole
  UTC days, at most 366 of them.
- `GET /api/v1/billing/status`: the plan of the session's active workspace,
  `{"plan": "free"|"basic"|"pro"|"agency", "status": "<Razorpay status>"|"none",
"currentPeriodEnd": RFC 3339|null, "cancelAtPeriodEnd": bool}`. Any member
  may read it.
- `POST /api/v1/billing/checkout` (`{"plan": "basic"|"pro"|"agency"}`):
  starts a subscription to that plan and returns
  `{"subscriptionId", "keyId"}` for Razorpay Checkout. Owners and admins
  only (`403` otherwise); `409` when the workspace already has a live
  subscription; `502` when Razorpay fails.
- `POST /api/v1/billing/cancel`: stops the subscription from charging again
  and returns the status. Owners and admins only; `409` when there is nothing
  to cancel. See `internal/billing/README.md`.
- `POST /webhooks/razorpay`: Razorpay subscription webhooks, verified by
  `X-Razorpay-Signature` (`401` when it is missing or wrong).
- `POST /api/v1/projects/{projectId}/ai-search/brand-lookup` and
  `.../ai-search/prompt-explorer`: AI Visibility lookups, `402` on the free
  plan when billing is configured, `503` without `DATAFORSEO_API_KEY`. See
  `internal/aisearch/README.md`.
- `POST /api/v1/projects/{projectId}/backlinks/overview`, `/rows`,
  `/referring-domains` and `/top-pages`: paid-plan gated provider lookups when
  billing is configured, `503` without `DATAFORSEO_API_KEY`. See
  `internal/backlinks/README.md`.
- `POST /api/v1/projects/{projectId}/domain/overview`, `/keyword-suggestions`,
  `/keywords` and `/pages`: Domain Overview lookups, same gating as backlinks.
  See `internal/domain/README.md`.
- `POST /api/v1/projects/{projectId}/site-connection/get`, `/save`,
  `/delete` and `.../content/drafts/{draftId}/publish`: connect the
  project's WordPress site and publish drafts to it; `402` below Basic when
  billing is configured. See `internal/sitepublish/README.md`.
- `/api/v1/...`: needs a valid session cookie, otherwise 401.
  `/api/v1/projects/{projectId}/...` also needs membership of the project's
  organization; any other project answers 404. Errors are JSON:
  `{"error": {"code": "...", "message": "..."}}`.
- Everything else is the React app: a file under `STATIC_DIR` is served as
  is (`assets/` immutable for a year), any other path without an extension
  gets `index.html` (`no-cache`, `frame-ancestors 'self'`), and a missing
  asset stays 404.
- `GET /r/{reportId}`: report viewer for signed-in members (report CSP,
  white-label bar, `?print=1`). `GET /s/{token}`: public share page that
  frames `/s/{token}/raw`. `GET /.well-known/openai-apps-challenge`: the
  `OPENAI_APPS_CHALLENGE` token.
- Billing: `GET /api/v1/billing/credits` (plan + credit balance),
  `GET /api/v1/billing/usage` (balance, this month's spend by feature,
  typical cost per action; `api/billing.yaml`),
  `POST /api/v1/billing/usage-events` (settled spends). A paid subscription
  charge grants the plan's `MonthlyCredits` once per webhook event. Every
  paid DataForSEO request and AI call spends credits through the spend
  guard (`internal/billing/README.md`); a refusal answers `402
  insufficient_credits` or `503 usage_paused`.

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

Basic, Pro and Agency, each sold as a Razorpay subscription per workspace
(organization). What they cost and include is in `internal/plans/README.md`.
More, including cancelling and the credit ledger, is in
`internal/billing/README.md`.
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
run at startup under a Postgres advisory lock. Version 1 holds the tables
the product started with; tables added since, and the `go_schema_migrations`
history, are prefixed `go_`. See `internal/database/README.md`.

## Checks (all mandatory, see /CLAUDE.md)

```sh
go mod tidy && gofmt -l . && go vet ./... && staticcheck ./... \
  && golangci-lint run ./... && go test -p 1 -race -count=1 -cover ./... \
  && govulncheck ./... && deadcode -test ./...
```

Integration tests need `TEST_DATABASE_URL` (Postgres) and `TEST_REDIS_URL`
(Redis). CI always sets them, and the tests fail there if one is missing.
