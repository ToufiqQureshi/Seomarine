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
| Live usage shown to the customer                                             | ⏳     |
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
| Billing              | Free and one Pro plan (₹1,499 a month) on Razorpay Subscriptions, webhooks, cancel, internal credit ledger |
| Privacy              | `cmd/erase-user` erases one account                                                           |

## Not built yet

| Feature                                                                 | Status |
| ----------------------------------------------------------------------- | ------ |
| The Basic / Pro / Agency plans decided below, with their limits         | ⏳     |
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
4. Build the three paid plans and their limits.

## Owner actions

- Razorpay: create the Pro plan and the webhook, put the four `RAZORPAY_*`
  values in Railway. Steps are in `docs/DEPLOY.md`.
- Submit the Google OAuth app for verification.
- Have a lawyer read the terms, privacy, refund and shipping pages.
- Make the GitHub repo private if it is not already.

Secrets go into Railway variables only, never into the repo or chat.

## Decided

- Name: Uplumo. Domain: uplumo.com. Hosting: Railway. Redis: yes.
- Launch market: India only, prices in INR. USD pricing comes later.
- GST: not registered at launch, so prices carry no tax line.
- Customers never see credits. Each plan has plain limits; the credit ledger
  stays internal, as a cap on what one workspace can cost us.
- Plans (owner, 12 October 2026). Not built yet: today the code has Free and
  one Pro plan at ₹1,499.

  | Limit               | Basic | Pro    | Agency             |
  | ------------------- | ----- | ------ | ------------------ |
  | Price per month     | ₹999  | ₹2,499 | ₹5,999             |
  | Sites               | 1     | 2      | 10, then ₹400 each |
  | Tracked keywords    | 100   | 500    | 2,000              |
  | White-label reports | no    | no     | yes                |
  | Client access       | no    | no     | yes                |
  | Team members        | 1     | 3      | 10                 |

  Sites and white-label are what separate Pro from Agency: a small agency
  with four client sites must not fit in Pro.

## Open decisions for the owner

- Whether a Free plan stays beside the three paid plans. Today the Free plan
  on the pricing page lists white-label reports, which the plan table above
  gives only to Agency.
- Data provider: keep DataForSEO, or drop the features that need it
  (keywords, rank tracking, backlinks, AI visibility). An AI model cannot
  replace it, because it has no real search data.
- Audit page and AI-check limits per plan.
- The old `/billing` page (credits and top-ups in USD) and the `/yc` offer
  page are still in the app and contradict "customers never see credits".
  Remove them or rebuild them.
- Cloudflare's managed `robots.txt` blocks AI crawlers (GPTBot, ClaudeBot,
  Google-Extended) on a product that sells AI visibility. Keep or turn off.
- Whether billing rows are kept when an account is erased (tax records). See
  `backend/internal/gdpr/README.md`.
