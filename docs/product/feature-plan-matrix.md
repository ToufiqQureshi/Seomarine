# Uplumo feature and plan matrix

**Decision draft, checked against this repository on 2026-10-13.** This is the
product inventory to use when deciding packaging. The server plan catalog and
route-level checks remain the enforcement source; this document does not make
a feature gated just by listing it here. “Current” describes behavior found in
code, not a promise that every external provider is configured in production.

## Plans in the current catalog

The current catalog contains a no-cost Free option and three paid plans. Prices
are monthly INR amounts in the server catalog.

| Plan | Price | Projects/sites | Intended customer | Cost boundary in current code |
|---|---:|---:|---|---|
| Free | ₹0 | 1 | Try Uplumo Analytics and connect a site | Paid SEO research and AI visibility are gated |
| Basic | ₹899 | 1 | One-site owner who needs first-party and Google search data | No paid SEO-data or AI/LLM provider calls |
| Pro | ₹1,899 | 5 | Growing business or consultant using research and AI features | Paid-provider access; catalog lists $3 monthly usage credits |
| Agency | ₹4,999 | 25 | Small agency handling multiple client sites | Pro features, white-label branding; catalog lists $10 monthly usage credits |

The catalog labels the Pro/Agency credits for AI writing and rendered-page
audits. The existing billing ledger and DataForSEO integrations also contain a
legacy provider-credit path. Confirm the production billing configuration and
what each credit balance pays for before advertising any allowance as a hard
spend cap. Do not promise unlimited provider usage or a fixed number of reports
until that accounting is unified and tested.

## Feature-by-feature packaging recommendation

| Feature / app area | What the user gets | Provider cost / dependency | Current gate or status | Recommended tier |
|---|---|---|---|---|
| Uplumo Analytics dashboard | First-party traffic overview and AI referral traffic | Uplumo-hosted collection and storage | Included in Free catalog; requires site tracking setup | Free, Basic, Pro, Agency |
| Google Search Console connection | Connect a verified Google property | Google OAuth and user-owned Search Console data; no paid SEO provider | Basic catalog includes connection; implementation is available | Basic+ |
| Search Performance | Clicks, impressions, CTR, position and query/page views | Search Console API; user account authorization | Basic catalog includes it; project must have a connected property | Basic+ |
| Google Analytics 4 | Connect and view GA4 property data | Google OAuth and GA4 API; user-owned data | Connection and dashboard flows exist; availability depends on account/property access | Basic+ |
| Keyword Research | Ideas and keyword metrics such as volume, difficulty, CPC, intent and SERP context | DataForSEO paid calls | Hosted paid-plan gate exists; a Basic plan must not be treated as paid access | Pro, Agency |
| Domain Overview / competitor research | Organic footprint and ranked keywords for a domain | DataForSEO paid calls | Hosted paid-plan gate exists | Pro, Agency |
| Backlink Research | Backlinks, referring domains and competitor link profiles | Paid backlink provider (including Ahrefs integration) | Hosted paid-plan gate exists | Pro, Agency |
| Rank Tracking | Save keywords and check rankings over time | DataForSEO checks; recurring schedules can incur repeat cost | Hosted paid-plan checks exist for checks and metric refresh | Pro, Agency; require explicit quotas/budget controls |
| Technical Site Audit | Crawl pages and report technical SEO issues | Uplumo crawler; rendered-page analysis adds external compute/provider cost | Hosted audit service has a paid-plan gate (Pro/Agency under current billing service); rendered checks use metering | Pro, Agency; consider a separately bounded no-render crawl for Basic as a future addition |
| AI Visibility: Brand Lookup | Brand mentions, citations, trends and share of voice | External LLM/search providers; variable paid usage | Paid-plan gate exists | Pro, Agency; publish safe usage limits |
| AI Visibility: Prompt Explorer | Run prompts against supported AI answer platforms | External LLM/search providers; variable paid usage | Paid-plan gate exists | Pro, Agency; publish safe usage limits |
| AI Writing Assistant / Content Editor | Draft or improve SEO content with AI | LLM provider calls | Writer endpoint requires paid plan; monthly credit semantics need alignment | Pro, Agency within a visible allowance |
| Saved Keywords | Save and organize keyword opportunities | Usually no new cost to view; research used to discover metrics costs provider usage | Save/research routes have mixed gates; keep saved-list reading low-cost | Basic for saved items; Pro+ for paid discovery and metrics |
| Project Context | Store brand, audience, competitor and key-page context for research | No direct provider cost | Available as a project workflow | Basic+ |
| Local SEO | Local keywords, Maps and local SERP research | DataForSEO paid calls | Provider-backed features should follow paid-plan gate; verify every endpoint | Pro, Agency |
| Reports | View, print/export saved reports | Storage; report creation may involve agent/provider workflows | App report viewing exists; MCP can save reports | Basic: view/export; Pro+: create/provider-backed workflows; Agency white-label |
| MCP / AI Agent setup | Connect a compatible agent to Uplumo SEO tools | Provider cost depends on tools invoked; OAuth/MCP infrastructure | Setup flow exists; tool-level paid checks apply unevenly | Basic: setup and zero-cost tools; Pro+ paid-data tools; Agency for client operations |
| White-label / branding | Apply agency identity to client-facing reports/surfaces | Low direct provider cost; support and configuration overhead | Custom branding handler is Agency-gated | Agency |
| Projects/sites | Separate work by website/client | Storage, crawl and scheduled provider usage scales with projects | Catalog caps are 1/1/5/25; verify every create/import path uses the server limit | As listed per plan |
| Team seats and roles | Invite collaborators and manage access | Support/security cost; no per-seat allowance defined in plan catalog | Team/organization flows exist; no tier seat matrix in catalog | Define before promising. Recommend Pro: 2 seats, Agency: 5 seats initially, with add-on decision |
| Billing and usage | View plan, credits and usage; cancel/manage subscription | Payment processor and support | Billing flows exist; active configuration differs by deployment | All plans; clearly show renewal, credit scope, exhaustion behavior and cancellation |
| Help, account and project settings | Account management, integrations, crawl access and project configuration | Mostly Uplumo infrastructure | Available according to account/project permissions | All plans, with paid integrations/features gated separately |

## Proposed customer-facing bundle

Use the following as the plain-language plan story while resolving the open
metering decisions above:

- **Basic — ₹899/month:** one website; Uplumo Analytics; Search Console and
  GA4 connections; search performance; saved keywords and project context.
  No keyword-volume/domain/backlink provider calls,
  rank checks, AI visibility, or AI writing. This keeps the entry plan useful
  without silently paying per query or per generated answer. A no-render,
  tightly bounded technical crawl could be added later if its server-side
  runtime and abuse limits are defined.
- **Pro — ₹1,899/month:** everything in Basic for up to five websites; keyword
  and domain research, backlinks, rank tracking, Local SEO, AI Visibility,
  AI writing and rendered-page audits. State provider quotas and credit
  exhaustion behavior beside the purchase button before launch.
- **Agency — ₹4,999/month:** everything in Pro for up to 25 websites; agency
  branding and client-ready reports. State seat limits and provider budget
  controls before launch.

Free remains a no-cost evaluation tier for one site and analytics basics; it
does not count as one of the three paid plans.

## Decisions and implementation gaps before launch

1. **One credit model:** reconcile the catalog’s AI-writing/rendered-audit
   allowances with the legacy DataForSEO credit ledger. Define whether paid
   SEO calls are included, separately metered or subject to per-feature quotas.
2. **Provider budgets:** add enforceable monthly/per-day limits and concurrency
   limits for DataForSEO and each AI provider. A “paid plan” boolean alone is
   not a spend ceiling.
3. **Audit tiers:** decide whether Basic gets no audit or a bounded no-render
   crawl. Current hosted audit behavior requires Pro/Agency; rendered-page
   analysis is metered. Verify API and background worker paths, not just UI
   visibility, if introducing a Basic audit.
4. **Site caps:** audit project creation, imports, invitations and downgrades
   against 1/1/5/25. Define what happens to existing sites when a customer
   downgrades below their current count.
5. **Seats:** decide and enforce member caps; current plan catalog has no seat
   entitlements. Avoid claiming unlimited seats until the policy is intentional.
6. **Endpoint coverage:** create one server-side entitlement map for UI, API,
   MCP and scheduled jobs. Confirm Local SEO, saved-keyword reads, reports and
   each agent tool follow the same policy.
7. **Plan names/prices in billing:** ensure payment-provider product/plan IDs,
   renewal amount, taxes and cancellation/refund copy match Free/Basic/Pro/
   Agency and ₹899/₹1,899/₹4,999.

## Source of truth

- Plan IDs, price, site counts and feature labels: `backend/internal/plans/catalog.go`.
- Public pricing copy: `backend/internal/site/templates/pricing.html` and
  `backend/internal/site/site.go`.
- Provider gates: handlers in `backend/internal/keywords`, `domain`,
  `backlinks`, `ranktracking`, `aisearch`, `contenteditor` and `audit`.
- Billing and usage ledger: `backend/internal/billing`.
- Recheck this document whenever catalog entitlements or provider billing
  behavior changes.
