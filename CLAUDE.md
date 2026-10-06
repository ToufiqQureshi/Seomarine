# Seomarine: product brief for agents

Read this before any work in this repo. Where this file conflicts with
`AGENTS.md` (inherited from OpenSEO), **this file wins**.

## What Seomarine is

An all-in-one SEO + AI-search platform that has to beat Semrush and Ahrefs
on the things users actually complain about, and not by cloning their
feature count.

Seomarine started as a fork of OpenSEO (MIT). The OpenSEO code is a
**reference and starting point, not the product**. Nothing users can see
may look or read like OpenSEO.

## Why people will pick us over Semrush and Ahrefs

These are the complaints we exist to fix (2026 research):

1. **Price.** A Semrush Pro seat is about $140/mo, and the AI Visibility
   add-on costs another $99/mo per domain. Ahrefs Lite caps every report
   at 2,500 rows and charges by credits. → We give simple, honest pricing,
   include AI visibility in the base plan, put no row caps on our own data,
   and keep INR pricing for India.
2. **Overwhelming UI.** Both tools bury solo owners and beginners under
   features. → We give focused workflows with one clear next action per
   screen, plus a plain-language explanation and an "AI fix" for every
   finding.
3. **Billing dark patterns.** Users report surprise price hikes, two-step
   cancellation and credit confusion. → We cancel in one click, show live
   usage, and never surprise anyone on price.
4. **AI search is the new SEO.** Over 40% of Google searches show AI
   Overviews, and AI-referred visitors convert 4–11x better than organic
   ones. Yet about 70% of AI traffic lands as "Direct" in analytics. →
   We track both sides: where the brand shows up in AI answers, and who
   actually arrives from AI.

## Must-have features (priority order)

Full list and status: `ROADMAP.md`.

1. **Seomarine Analytics**: our own privacy-friendly tracker (GA4/PostHog
   style) with first-class **AI traffic detection** for ChatGPT,
   Perplexity, Gemini, Claude and Copilot, including recovery of the
   "Direct" traffic that is really AI.
2. **AI Visibility**: brand mentions, citations, sentiment and competitor
   share of voice across ChatGPT, Perplexity, Gemini, Google AI Overviews
   and Claude, joined with the analytics data so users see "seen in AI"
   next to "visitors from AI".
3. **Content SEO Editor ("RankMath for every site")**: works on any site
   (HTML, React, Webflow, Shopify, WordPress). It gives a live SEO score
   and AI rewrites, and ships fixes through a JS snippet, a GitHub PR, or
   copy-paste.
4. **White-label client reports**: done. Every client-facing surface is
   agency-branded.
5. Rank tracking with city-level India locations, a site audit with AI
   fix steps (English + Hinglish), and Local SEO with a Google Maps rank
   grid.

The bar for any feature: a solo owner gets value in under 5 minutes, and
an agency can show it to a client.

## Architecture (owner decision)

- **Go is the core.** Every new backend service is written in Go: API,
  analytics ingest, crawlers, schedulers and data pipelines.
- **No Next.js, and no Node/JS backend for new work.** The existing
  TypeScript/TanStack/Cloudflare Workers backend is legacy. Port it to Go
  feature by feature; don't add new backend features to it.
- **Frontend:** a React SPA built with Vite and served by the Go server
  as static files (no SSR framework). Existing React components can be
  reused once rebranded.
- **Data:** Postgres is the primary store. Use a column store
  (ClickHouse) for analytics events once volume needs it. Keep relational
  data normalized.
- **Go conventions:** standard library first (`net/http`, `log/slog`,
  `context`). The layering is handler → service → repository. Use
  `sqlc` or plain SQL, not a heavy ORM. Validate every input at the
  handler. Return errors and never panic for control flow. Run
  `go test ./...`, `go vet` and `golangci-lint` before every push.
- **External data:** DataForSEO stays the SEO data provider (BYOK on
  self-host). Isolate it behind one Go client package so it can be swapped.

## UI and brand rules

- Ship a fresh design system: our own name, logo, palette, typography,
  icons, layout and copy. Never use OpenSEO's look.
- Remove every user-visible "OpenSEO" string, link, logo, meta tag,
  email template and doc reference (except license attribution).
- Use plain language, and Hinglish where the market is India. Never show
  raw jargon without a one-line explanation.

## Working with the owner

- The owner (Toufiq) decides product scope, pricing, branding and
  architecture. Propose with a recommendation; don't decide silently.
- Keep answers short, in Hinglish, senior-dev tone.
- Ship each feature as its own PR with validation notes. Never commit
  secrets.
- Before planning a new feature, search the web for current competitor
  features, pricing and user complaints, and cite the sources in the PR
  or plan.

## Legal

- `LICENSE` is proprietary (all rights reserved) for Seomarine's own work.
- OpenSEO-derived code stays under MIT. Never delete
  `LICENSE-OPENSEO-MIT` or the attribution it carries.
