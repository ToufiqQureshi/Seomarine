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
  handler. Return errors and never panic for control flow. The full check list
  is in **Code quality** below.
- **External data:** DataForSEO stays the SEO data provider (BYOK on
  self-host). Isolate it behind one Go client package so it can be swapped.

## Code quality: the most critical rule (mandatory, no exceptions)

Write code like a senior engineer shipping to production. Every line has
to be correct, needed and readable. Delete any line that doesn't earn its
place.

**Use what Go already gives you.** Before writing a helper, check the
standard library (`slices`, `maps`, `strings`, `errors`, `context`,
`net/http`, `encoding/json`, `log/slog`, `sync`, `time`, `testing`) and
the dependencies already in `go.mod`. Hand-rolling something the stdlib
already does counts as a defect.

**Every one of these must pass before any commit or push.** CI runs the
same list, and a failure blocks the merge:

| Command                                                                                                                                                                                                       | What it catches                                                                |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| `gofmt -l .` (must print nothing) and `goimports`                                                                                                                                                             | formatting, import order                                                       |
| `go vet ./...`                                                                                                                                                                                                | suspicious code: bad printf args, copied locks, unreachable code               |
| `staticcheck ./...`                                                                                                                                                                                           | bugs, deprecated APIs, simplifications                                         |
| `golangci-lint run` with `errcheck`, `unused`, `ineffassign`, `govet`, `staticcheck`, `gosec`, `bodyclose`, `sqlclosecheck`, `rowserrcheck`, `errorlint`, `nilerr`, `contextcheck`, `noctx`, `revive` enabled | ignored errors, dead and unused code, unclosed bodies and rows, security holes |
| `go test -race -count=1 ./...`                                                                                                                                                                                | real failures and data races                                                   |
| `go test -cover ./...`                                                                                                                                                                                        | untested code paths in the packages you changed                                |
| `govulncheck ./...`                                                                                                                                                                                           | known vulnerabilities in dependencies                                          |
| `go mod tidy` (`go.mod`/`go.sum` must not change)                                                                                                                                                             | unused or missing modules                                                      |
| `deadcode ./...`                                                                                                                                                                                              | functions nothing calls                                                        |

**Tests exist to catch bugs, not to turn CI green.**

- Every test must be able to fail. Test the real behavior, the edge
  cases (empty, nil, max size, unicode, timeouts, concurrent access) and
  the error paths that can actually happen.
- Prefer table-driven tests. Use `httptest` for handlers and a real
  Postgres (testcontainers or a CI service) for repositories. Never mock
  SQL.
- When a bug is fixed, add the test that reproduces it first, see it
  fail, then fix the code.
- A failing test is a real bug until proven otherwise. Never skip,
  weaken or delete a test to get green.

**Errors and safety**

- Never ignore an error. Wrap it with context:
  `fmt.Errorf("load project %s: %w", id, err)`. Check it with
  `errors.Is` or `errors.As`.
- Pass `context.Context` through every request path, and set a timeout
  on every outbound call.
- No global mutable state. Make concurrency explicit with `sync` or
  channels, and keep it race-free under `-race`.
- Validate all untrusted input at the handler boundary.

**The frontend gets the same rigor.** `tsc --noEmit`, `oxlint`, `knip`
(no unused exports or files), `prettier --check` and the tests must all
pass.

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
