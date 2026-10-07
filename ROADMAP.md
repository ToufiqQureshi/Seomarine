# Seomarine roadmap

Status: ✅ merged · 🔀 built on a branch, waiting for its PR · 🔨 in progress · ⏳ pending.
One PR per item.
Product reasoning lives in `CLAUDE.md`.

## Phase 0: Foundation (must come first)

| #    | Task                                                                                                                            | Status |
| ---- | ------------------------------------------------------------------------------------------------------------------------------- | ------ |
| 0.1  | Import code into the Seomarine repo, add the proprietary license, keep the MIT attribution                                      | ✅     |
| 0.2  | Make the GitHub repo private (owner does this in GitHub settings)                                                               | ⏳     |
| 0.3  | Go backend skeleton: config, Postgres pool, health/readiness, graceful shutdown, CI with all mandatory checks                   | ✅     |
| 0.3b | Migrations (goose) + auth (reads the existing login session), Redis, proxy to the legacy app (`go-mvp`)                         | 🔀     |
| 0.4  | Go server serves the React SPA (Vite build) as static files                                                                     | ⏳     |
| 0.5  | Plan the port of existing TypeScript backend features to Go (projects, keywords, rank tracking, audit, backlinks, reports, MCP) | ⏳     |

## Phase 1: Rebrand + new UI (so nobody can tell it's an OpenSEO fork)

| #   | Task                                                                      | Status |
| --- | ------------------------------------------------------------------------- | ------ |
| 1.1 | Brand kit: logo, colors, typography, icon set (`rebrand-ui`)              | 🔀     |
| 1.2 | New app shell: sidebar, navigation, dashboard layout (`rebrand-ui`)       | 🔀     |
| 1.3 | Remove every user-visible "OpenSEO" string, link, meta tag, email and doc | 🔀     |
| 1.4 | New landing page + INR/USD pricing page (`go-mvp`)                        | 🔀     |
| 1.5 | Plain-language and Hinglish copy pass                                     | ⏳     |

## Phase 2: Core differentiators

| #   | Feature                                                                                                                                                       | Status |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ |
| 2.1 | White-label PDF/share reports (logo, name, color, website)                                                                                                    | ✅     |
| 2.2 | **Seomarine Analytics** (Go): tracking script, ingest API, dashboard (visitors, pages, sources, devices, countries)                                           | 🔀     |
| 2.3 | **AI traffic detection**: ChatGPT, Perplexity, Gemini, Claude, Copilot referrers, plus recovery of AI traffic hiding as "Direct"                              | 🔀     |
| 2.4 | **AI Visibility**: mentions, citations, sentiment and competitor share of voice across ChatGPT, Perplexity, Gemini, AI Overviews and Claude, with drop alerts | ⏳     |
| 2.5 | Join AI visibility with analytics: "seen in AI" vs "visitors from AI"                                                                                         | ⏳     |
| 2.6 | **Content SEO Editor** ("RankMath for every site"): live score, keyword/heading/meta/readability checks, AI rewrite                                           | ⏳     |
| 2.7 | Content editor fix delivery: JS snippet, GitHub PR, copy-paste                                                                                                | ⏳     |

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

| #   | Task                                                                      | Status |
| --- | ------------------------------------------------------------------------- | ------ |
| 5.1 | Production hosting on Railway: Go server + Postgres + Redis               | 🔨     |
| 5.2 | Domain, email, payments (Razorpay subscriptions built on `go-mvp`)        | 🔨     |
| 5.3 | Simple pricing with AI visibility included, no row caps, one-click cancel | ⏳     |

## Where the unmerged work lives

| Branch       | Contents                                                                                                           | Next step                                                     |
| ------------ | ------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------- |
| `rebrand-ui` | Seomarine brand, new app shell, no OpenSEO strings, analytics dashboard, billing plan page                         | Open the PR, CI green, merge                                  |
| `go-mvp`     | Migrations, Redis, session auth, legacy proxy, analytics ingest + AI traffic, Razorpay subscriptions, landing page | Railway deploy files (Dockerfile, `railway.json`), review, PR |

`rebrand-ui` calls the Go API from `go-mvp`, so merge `go-mvp` first or
together.

## Owner actions

- Make the GitHub repo private (0.2).
- Buy the domain, create the Razorpay account and the Railway project.
  Secrets go into Railway variables only, never into the repo or chat.

## Decided

- Name: Seomarine. Hosting: Railway. Redis: yes.

## Open decisions for the owner

- Domain.
- Pricing tiers (INR and USD).
