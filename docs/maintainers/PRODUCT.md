# Seomarine: product brief

Why Seomarine exists, who it beats and what must ship first. Read it before
choosing, prioritising or scoping a feature. Rules for writing code live in
`CLAUDE.md`; feature status lives in `ROADMAP.md`.

## What Seomarine is

An all-in-one SEO + AI-search platform that has to beat Semrush and Ahrefs
on the things users actually complain about, and not by cloning their
feature count.

Part of the codebase is derived from an MIT-licensed project (see
`LICENSE-OPENSEO-MIT`). That code is a **starting point, not the
product**. Nothing in the product, code or docs may carry the upstream
name, links, look or services.

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
