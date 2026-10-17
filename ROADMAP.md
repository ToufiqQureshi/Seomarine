# Uplumo roadmap

Status: ✅ done · 🔨 in progress · ⏳ not started.
Product reasoning lives in `docs/PRODUCT.md`; rules for agents in `CLAUDE.md`.

Last reviewed against the code, Railway and the live site on 13 October 2026.

## Where we are

Uplumo is live at `https://uplumo.com` on Railway. The whole backend is Go and
the React app is a static SPA the Go server serves. People can sign up, verify
their email and use the free features. **Nobody pays real money yet**:
Razorpay runs with test-mode keys in production, so checkout works only with
test cards. DataForSEO and OpenRouter keys are not in production.

## Content editor

- Built: visual editor, local SEO/AEO scoring, search briefs (Brief tab,
  `POST /api/v1/projects/{projectId}/content/brief`, shared 30-day cache),
  13 AI writing actions with per-action credit ceilings, publish to
  WordPress, and "Track this article" links to Rank Tracking and Prompt
  Explorer after publishing. The free editor pack also adds project author
  profiles, review dates, WordPress Article/FAQ schema where permitted, and
  internal links from Search Console or the latest audit.
- Not built: scheduled tracking of a published URL's position and AI
  mentions inside the editor (today the writer opens Rank Tracking and
  Prompt Explorer from the publish card).

## Launch checklist

| Item                                                                                                   | Status                                 |
| ------------------------------------------------------------------------------------------------------ | -------------------------------------- |
| Railway deploy, domain, `www` redirect                                                                 | ✅                                     |
| Sign-up, email verification, password reset, Google sign-in                                            | ✅                                     |
| `support@uplumo.com` (Cloudflare forwards it)                                                          | ✅                                     |
| Loops email: verify and reset working; invitation template made, not tested                            | 🔨                                     |
| Public pages: landing, pricing, about, contact, terms, privacy, refunds, shipping                      | ✅                                     |
| Razorpay account approved (KYC)                                                                        | ✅                                     |
| Razorpay test-mode keys in Railway                                                                     | ✅                                     |
| Razorpay test payment, renewal and cancel through the site, then live keys                             | ⏳                                     |
| One-click cancel (`POST /api/v1/billing/cancel`, "Cancel Pro" on Billing)                              | ✅ built, not yet run against Razorpay |
| DataForSEO key in production (keywords, rank tracking, backlinks, AI visibility answer 503 without it) | ⏳                                     |
| Google OAuth app verification (Search Console and Analytics scopes)                                    | ⏳                                     |
| Live usage shown to the customer (credits on Billing, cost next to paid actions)                       | ✅ built                               |
| Spend guard: credits held before every paid lookup and AI call, daily limit                            | ✅ built                               |
| Lawyer review of the legal pages                                                                       | ⏳                                     |

## What is built

Built means the Go API and the page exist and are tested in CI. It does not
mean each one has been tried on production with real provider keys.

| Area                 | What exists                                                                                                                                                                                |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Accounts             | Email and password, verification, reset, Google sign-in, workspaces, invitations, roles, API keys                                                                                          |
| Uplumo Analytics     | Tracking script, ingest, dashboard (visitors, pages, sources, devices, countries), button clicks via `data-uplumo-event`                                                                   |
| AI traffic detection | ChatGPT, Perplexity, Gemini, Claude, Copilot referrers, plus AI traffic hiding as "Direct"                                                                                                 |
| AI Visibility        | Brand Lookup (mentions, citations, share of voice) and Prompt Explorer                                                                                                                     |
| Keywords             | Research, SERP analysis, saved keywords with tags                                                                                                                                          |
| Rank tracking        | Configs, manual and scheduled checks, city-level locations                                                                                                                                 |
| Site audit           | Crawl, issues, Lighthouse through Google PageSpeed Insights (free; homepage only on Free) with real-visitor Core Web Vitals                                                                |
| Backlinks, Domain    | Overview, rows, referring domains, top pages; domain keywords and pages                                                                                                                    |
| Google               | Search Console performance and URL inspection, GA4 reports and measurement health                                                                                                          |
| Reports              | White-label reports (logo, name, colour, website), public share links                                                                                                                      |
| Content editor       | Project drafts, live SEO/AEO checks, AI writing actions                                                                                                                                    |
| Website publishing   | Connect a WordPress site (onboarding or Settings → Integrations), publish or update drafts with their images (Basic and above)                                                             |
| Local SEO            | Business search, profile, Google Maps rank grid                                                                                                                                            |
| Assistant (SAM)      | Chat with tools over the project's data                                                                                                                                                    |
| MCP server           | Tools for Claude, ChatGPT and Cursor, with OAuth                                                                                                                                           |
| Billing              | Free, Basic, Pro and Agency on Razorpay Subscriptions, with site, tracked-keyword and member limits; webhooks, cancel, monthly credits with a spend guard on every paid lookup and AI call |
| Privacy              | `cmd/erase-user` erases one account                                                                                                                                                        |

## Not built yet

| Feature                                                                                  | Status |
| ---------------------------------------------------------------------------------------- | ------ |
| Changing from one paid plan to another without writing to support                        | ⏳     |
| Agency: client access, and extra sites at ₹400 each                                      | ⏳     |
| Live usage on the Billing page                                                           | ⏳     |
| AI Visibility: sentiment, tracking over time, drop alerts                                | ⏳     |
| "Seen in AI" next to "visitors from AI" (AI Visibility joined with Analytics)            | ⏳     |
| Publishing to Shopify, Wix, Webflow, plain HTML (SFTP) and an Uplumo-hosted blog         | ⏳     |
| Content editor on an external site; delivering fixes (JS snippet, GitHub PR, copy-paste) | ⏳     |
| Site audit: AI "how to fix" for every issue (English + Hinglish)                         | ⏳     |
| Scheduled reports, emailed to clients                                                    | ⏳     |
| Client portal: clients log in and see only their own reports                             | ⏳     |
| Google Business Profile audit and review tracking                                        | ⏳     |
| Plain-language and Hinglish copy pass across the app                                     | ⏳     |
| USD pricing                                                                              | ⏳     |
| Account deletion and data export from Settings                                           | ⏳     |
| SEO Change Guard: snapshot crawl results, flag SEO-critical changes, show affected URLs and verify fixes after the next crawl | ⏳ proposed |

## Competitive scan and product opportunity (16 October 2026)

This is a shortlist for product discovery, not a claim that competitors lack these capabilities. Public feature pages are not exhaustive.

- Ahrefs advertises always-on site audits, technical issue reporting and AI-assisted patches ([Site Audit](https://ahrefs.com/use-cases/seo), [product update](https://ahrefs.com/blog/new-features-apr-2025/)). Semrush also promotes technical audits and AI-crawler checks ([Site Audit](https://www.semrush.com/features/site-audit/)). So audits, internal links and AI-bot checks alone are not a defensible unique claim.
- Ahrefs AI Content Helper offers content-gap recommendations and in-editor assistance ([product page](https://ahrefs.com/ai-content-helper)). Content scoring and AI writing are also competitive categories; Uplumo should differentiate through its India-first pricing and a simpler action workflow, not claim exclusivity.
- **Proposed: SEO Change Guard.** Save crawl snapshots and diff titles, canonicals, indexability, robots directives, schema and internal links. Show the exact URLs and dates affected, then recrawl after the owner fixes the change. Use Uplumo's crawler and stored snapshots; this needs no paid SEO-data API.
- **Potential extension:** connect the change timeline to Uplumo's first-party analytics to show whether a change coincided with visits or lead events moving. Treat this as correlation, not proof that the change caused the movement.


## Next up

1. Take a Razorpay test payment through the site, cancel it, then switch to
   live keys.
2. Put the DataForSEO key in production, or hide the features that need it.
3. Walk through every page on production as a real user and fix what breaks.
4. Let a customer change plan from Billing.

## Owner actions

- Razorpay: create the three plans and the webhook, put the `RAZORPAY_*`
  values in Railway. Steps are in `docs/DEPLOY.md`.
- Submit the Google OAuth app for verification.
- Have a lawyer read the terms, privacy, refund and shipping pages.
- Make the GitHub repo private if it is not already.

Secrets go into Railway variables only, never into the repo or chat.

## Decided

- Name: Uplumo. Domain: uplumo.com. Hosting: Railway. Redis: yes.
- Launch market: India only, prices in INR. USD pricing comes later.
- GST: not registered at launch, so prices carry no tax line.
- Credits (owner, 13 October 2026; public pricing clarified 15 October 2026).
  Every paid data lookup and AI call spends credits; customers see each plan's
  monthly grant on the pricing page and see their balance and estimated action
  costs in the app.
  Monthly credits: Basic 4,000, Pro 10,000, Agency 25,000 (a credit is 1/1280
  of a dollar of provider cost). No credits means no request is sent. Each
  workspace may spend at most a third of its monthly credits in a day, so one
  heavy client never stops the others; `PROVIDER_DAILY_LIMIT_USD` (default
  $100) is only an emergency stop for a bug. Rules:
  `backend/internal/billing/README.md`. Unused credits roll over up to two
  months. Rank tracking depth is capped per plan (Basic top 30, Pro and
  Agency top 40) so weekly checks of every tracked keyword fit the credits.
- Plans (owner, 12 October 2026, confirmed 13 October). Built: the table
  below is `backend/internal/plans/catalog.go`. Free is one site with
  analytics and a starter audit.

  | Limit               | Basic | Pro    | Agency             |
  | ------------------- | ----- | ------ | ------------------ |
  | Price per month     | ₹999  | ₹2,499 | ₹5,999             |
  | Sites               | 1     | 2      | 10, then ₹400 each |
  | Tracked keywords    | 100   | 500    | 1,500              |
  | White-label reports | no    | no     | yes                |
  | Client access       | no    | no     | yes                |
  | Team members        | 1     | 3      | 10                 |

  Sites and white-label are what separate Pro from Agency: a small agency
  with four client sites must not fit in Pro. Basic and Pro differ by
  feature: Basic is keywords, rankings, Google data, search briefs and AI
  writing for one site; Pro adds backlinks, competitor research and AI
  Visibility.

- The owner's and the team's own workspaces are never limited by a plan
  (`STAFF_EMAILS`).

## Open decisions for the owner

- The typical costs on Billing (`backend/internal/billing/costs.go`) are from
  published prices; check them against the first real DataForSEO and
  OpenRouter invoices.
- Data provider: keep DataForSEO, or drop the features that need it
  (keywords, rank tracking, backlinks, AI visibility). An AI model cannot
  replace it, because it has no real search data.
- Audit page and AI-check limits per plan.
- The `/yc` offer page still talks about USD credits and top-ups. Remove it
  or rebuild it (the `/billing` page now shows credits from the Go API).
- Cloudflare's managed `robots.txt` blocks AI crawlers (GPTBot, ClaudeBot,
  Google-Extended) on a product that sells AI visibility. Keep or turn off.
- Whether billing rows are kept when an account is erased (tax records). See
  `backend/internal/gdpr/README.md`.
