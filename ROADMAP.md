# Uplumo roadmap

Status: ✅ done · 🔨 in progress · ⏳ not started.
Product reasoning lives in `docs/PRODUCT.md`; rules for agents in `CLAUDE.md`.

Last reviewed against the code and the live site on 13 October 2026.

## Where we are

Uplumo is live at `https://uplumo.com` on Railway. The whole backend is Go and
the React app is a static SPA the Go server serves. People can sign up, verify
their email and use the free features. **Nobody can pay yet**: Razorpay is
approved, but its keys are not in production.

## Launch checklist

| Item                                                                         | Status |
| ---------------------------------------------------------------------------- | ------ |
| Railway deploy, domain, `www` redirect                                       | ✅     |
| Sign-up, email verification, password reset, Google sign-in                  | ✅     |
| `support@uplumo.com` (Cloudflare forwards it)                                | ✅     |
| Loops email: verify and reset working; invitation template made, not tested  | 🔨     |
| Public pages: landing, pricing, about, contact, terms, privacy, refunds, shipping | ✅ |
| Razorpay account approved (KYC)                                              | ✅     |
| Razorpay test payment through the site, then live keys in Railway            | ⏳     |
| One-click cancel (`POST /api/v1/billing/cancel`, "Cancel Pro" on Billing)    | ✅ built, not yet run against Razorpay |
| DataForSEO key in production (keywords, rank tracking, backlinks, AI visibility answer 503 without it) | ⏳ |
| Google OAuth app verification (Search Console and Analytics scopes)          | ⏳     |
| Live usage shown to the customer (credits on Billing, cost next to paid actions) | ✅ built |
| Spend guard: credits held before every paid lookup and AI call, daily limit   | ✅ built |
| Lawyer review of the legal pages                                             | ⏳     |

## What is built

Built means the Go API and the page exist and are tested in CI. It does not
mean each one has been tried on production with real provider keys.

| Area                 | What exists                                                                                   |
| -------------------- | --------------------------------------------------------------------------------------------- |
| Accounts             | Email and password, verification, reset, Google sign-in, workspaces, invitations, roles, API keys |
| Uplumo Analytics     | Tracking script, ingest, dashboard (visitors, pages, sources, devices, countries)             |
| AI traffic detection | ChatGPT, Perplexity, Gemini, Claude, Copilot referrers, plus AI traffic hiding as "Direct"    |
| AI Visibility        | Brand Lookup (mentions, citations, share of voice) and Prompt Explorer                        |
| Keywords             | Research, SERP analysis, saved keywords with tags                                             |
| Rank tracking        | Configs, manual and scheduled checks, city-level locations                                    |
| Site audit           | Crawl, issues, Lighthouse                                                                     |
| Backlinks, Domain    | Overview, rows, referring domains, top pages; domain keywords and pages                       |
| Google               | Search Console performance and URL inspection, GA4 reports and measurement health             |
| Reports              | White-label reports (logo, name, colour, website), public share links                         |
| Content editor       | Project drafts, live SEO/AEO checks, AI writing actions                                       |
| Local SEO            | Business search, profile, Google Maps rank grid                                               |
| Assistant (SAM)      | Chat with tools over the project's data                                                       |
| MCP server           | Tools for Claude, ChatGPT and Cursor, with OAuth                                              |
| Billing              | Free, Basic, Pro and Agency on Razorpay Subscriptions, with site, tracked-keyword and member limits; webhooks, cancel, internal credit ledger |
| Privacy              | `cmd/erase-user` erases one account                                                           |

## Not built yet

| Feature                                                                 | Status |
| ----------------------------------------------------------------------- | ------ |
| Changing from one paid plan to another without writing to support       | ⏳     |
| Agency: client access, and extra sites at ₹400 each                     | ⏳     |
| Live usage on the Billing page                                          | ⏳     |
| AI Visibility: sentiment, tracking over time, drop alerts               | ⏳     |
| "Seen in AI" next to "visitors from AI" (AI Visibility joined with Analytics) | ⏳ |
| Content editor on an external site; delivering fixes (JS snippet, GitHub PR, copy-paste) | ⏳ |
| Site audit: AI "how to fix" for every issue (English + Hinglish)        | ⏳     |
| Scheduled reports, emailed to clients                                   | ⏳     |
| Client portal: clients log in and see only their own reports            | ⏳     |
| Google Business Profile audit and review tracking                       | ⏳     |
| Plain-language and Hinglish copy pass across the app                    | ⏳     |
| USD pricing                                                             | ⏳     |
| Account deletion and data export from Settings                          | ⏳     |

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
- Credits (owner, 13 October 2026). Every paid data lookup and AI call
  spends credits; customers see them in the app (Billing, and "about N
  credits" next to each paid action) but not on the public pricing page.
  Monthly credits: Basic 4,000, Pro 10,000, Agency 25,000 (a credit is 1/1280
  of a dollar of provider cost). No credits means no request is sent. A
  daily limit on all provider spend (`PROVIDER_DAILY_LIMIT_USD`, default $25)
  pauses paid work for everyone as the last safety net. Rules:
  `backend/internal/billing/README.md`.
- Plans (owner, 12 October 2026, confirmed 13 October). Built: the table
  below is `backend/internal/plans/catalog.go`. Free is one site with
  analytics and a starter audit.

  | Limit               | Basic | Pro    | Agency             |
  | ------------------- | ----- | ------ | ------------------ |
  | Price per month     | ₹999  | ₹2,499 | ₹5,999             |
  | Sites               | 1     | 2      | 10, then ₹400 each |
  | Tracked keywords    | 100   | 500    | 2,000              |
  | White-label reports | no    | no     | yes                |
  | Client access       | no    | no     | yes                |
  | Team members        | 1     | 3      | 10                 |

  Sites and white-label are what separate Pro from Agency: a small agency
  with four client sites must not fit in Pro. Basic and Pro differ by
  feature: Basic is keywords, rankings and Google data for one site; Pro
  adds backlinks, competitor research, AI Visibility and AI writing.
- The owner's and the team's own workspaces are never limited by a plan
  (`STAFF_EMAILS`).

## Open decisions for the owner

- Credits on the public pricing page: today only in the app. Showing "10,000
  credits a month" would set expectations; hiding it avoids explaining them.
- Unused credits roll over every month. Cap the rollover (for example at two
  months) or reset each month.
- Pro's 500 tracked keywords checked weekly at top-100 depth cost about 12,800
  credits a month, more than Pro's 10,000. Top-30 fits easily. Either raise
  Pro's credits, cap the depth, or let customers choose with the estimate.
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
