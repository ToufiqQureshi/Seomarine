# Seomarine roadmap

Status: ✅ merged · 🔨 in progress · ⏳ pending.
One PR per item.
Product reasoning lives in `docs/maintainers/PRODUCT.md`; rules for agents in `CLAUDE.md`.

## Phase 0: Foundation (must come first)

| #    | Task                                                                                                                                                    | Status      |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------- |
| 0.1  | Import code into the Seomarine repo, add the proprietary license, keep the MIT attribution                                                              | ✅          |
| 0.2  | Make the GitHub repo private (owner does this in GitHub settings)                                                                                       | ⏳          |
| 0.3  | Go backend skeleton: config, Postgres pool, health/readiness, graceful shutdown, CI with all mandatory checks                                           | ✅          |
| 0.3b | Migrations (goose) + auth (reads the existing login session), Redis, proxy to the legacy app                                                            | ✅          |
| 0.4  | Go server serves the React SPA (Vite build) as static files                                                                                             | ⏳          |
| 0.5  | Plan the port of existing TypeScript backend features to Go (projects, keywords, rank tracking, audit, backlinks, reports, MCP)                         | 🔧          |
| 0.5a | MCP Go dispatcher and TypeScript fallback; tool catalog, read-only and billed tool ports remain                                                         | in progress |
| 0.5d | Port keyword/rank-tracking market tables and resolution to Go; API and jobs remain pending                                                              | in progress |
| 0.5f | Rank tracking in Go: tables, rules, configs, keywords, results, manual/scheduled checks and metrics refresh done; credit holds and React switch pending | in progress |
| 0.5g | Keywords in Go: saved keywords, tags, research (Labs/Ads, local), SERP analysis, metrics refresh; React switch pending                                  | in progress |
| 0.5b | Backlinks reports API in Go; React uses Go endpoints while MCP remains on TypeScript                                                                    | in progress |
| 0.5c | Domain overview API in Go (overview, keyword suggestions, keywords and pages tabs); React uses it, MCP stays on TypeScript                              | in progress |
| 0.5e | SERP location search API in Go (city and region picker, 30-day country cache); React uses it, MCP stays on TypeScript                                   | in progress |
| 0.5h | GA4 reports, organic overview, measurement-health, Search Opportunity and property setup APIs in Go; MCP and report-page React switch pending           | in progress |
| 0.5i | GSC Search Performance API in Go (report, query/page paging and export); MCP and React switch pending                                                   | in progress |
| 0.5j | GSC project connection API in Go (grant status, property listing/selection, disconnect, URL inspection); OAuth, MCP and React switch pending            | in progress |
| 0.5k | SAM session registry API in Go; chat runtime, tools, metering and React switch pending                                                                  | in progress |

## Phase 1: Rebrand + new UI

| #   | Task                                                                    | Status |
| --- | ----------------------------------------------------------------------- | ------ |
| 1.1 | Brand kit: logo, colors, typography, icon set                           | ✅     |
| 1.2 | New app shell: sidebar, navigation, dashboard layout                    | ✅     |
| 1.3 | Remove every trace of the upstream brand (code, docs, links, telemetry) | ✅     |
| 1.4 | New landing page + INR pricing page (USD pending)                       | ✅     |
| 1.5 | Plain-language and Hinglish copy pass                                   | ⏳     |

## Phase 2: Core differentiators

| #    | Feature                                                                                                                                                       | Status |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ |
| 2.1  | White-label PDF/share reports (logo, name, color, website)                                                                                                    | ✅     |
| 2.2  | **Seomarine Analytics** (Go): tracking script, ingest API, dashboard (visitors, pages, sources, devices)                                                      | ✅     |
| 2.2b | Analytics: countries breakdown                                                                                                                                | 🔨     |
| 2.3  | **AI traffic detection**: ChatGPT, Perplexity, Gemini, Claude, Copilot referrers, plus recovery of AI traffic hiding as "Direct"                              | ✅     |
| 2.4  | **AI Visibility**: mentions, citations, sentiment and competitor share of voice across ChatGPT, Perplexity, Gemini, AI Overviews and Claude, with drop alerts | 🔨     |
| 2.4a | AI Visibility base in Go: Brand Lookup (mentions, citations, share of voice) and Prompt Explorer, ported from the legacy app                                  | ✅     |
| 2.5  | Join AI visibility with analytics: "seen in AI" vs "visitors from AI"                                                                                         | ⏳     |
| 2.6  | **Content SEO Editor** ("RankMath for every site"): live score, keyword/heading/meta/readability checks, AI rewrite                                           | ⏳     |
| 2.7  | Content editor fix delivery: JS snippet, GitHub PR, copy-paste                                                                                                | ⏳     |

## Phase 3: Improvements to existing features

| #   | Feature                                                          | Status |
| --- | ---------------------------------------------------------------- | ------ |
| 3.1 | Rank tracking: India city-level locations                        | ⏳     |
| 3.2 | Site audit: AI "how to fix" for every issue (English + Hinglish) | ⏳     |
| 3.3 | Scheduled reports, auto-emailed to clients                       | ⏳     |
| 3.4 | Client portal: clients log in and see only their own reports     | ⏳     |

## Phase 4: Local SEO

| #   | Feature                                           | Status |
| --- | ------------------------------------------------- | ------ |
| 4.1 | Google Maps rank grid ("dentist near me" by area) | ⏳     |
| 4.2 | Google Business Profile audit + review tracking   | ⏳     |

## Phase 5: Launch

| #   | Task                                                              | Status |
| --- | ----------------------------------------------------------------- | ------ |
| 5.1 | Railway deploy: Dockerfile, `railway.json`, deploy guide, go live | ⏳     |
| 5.2 | Domain, email, payments (Razorpay checkout + webhooks done in Go) | 🔨     |
| 5.3 | One-click cancel, live usage, AI visibility in the base plan      | ⏳     |

## Next up (engineering)

1. 5.1 Railway deploy, so the product is live.
2. 5.3 One-click cancel and live usage (part of the pricing promise).
3. 0.4 Go serves the React build, retiring the legacy proxy route by route.
4. 2.4 AI Visibility, then 2.5 joining it with analytics.

Backlinks Go API is implemented for the React pages. Legacy MCP tools continue
using the TypeScript service until MCP is ported.

## Owner actions

- Make the GitHub repo private (0.2).
- Buy the domain, create the Razorpay account and the Railway project.
  Secrets go into Railway variables only, never into the repo or chat.

## Decided

- Name: Seomarine. Hosting: Railway. Redis: yes.

## Open decisions for the owner

- Domain.
- Pricing tiers (INR and USD).
