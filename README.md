# Uplumo

SEO and AI-search platform, live at <https://uplumo.com>: keyword research,
rank tracking, site audits, backlinks, white-label client reports, and
analytics that shows which AI assistants (ChatGPT, Perplexity, Gemini, Claude,
Copilot) send you visitors.

Proprietary software. See [`LICENSE`](./LICENSE).

## Where things live

| Path                     | What                                                             |
| ------------------------ | ---------------------------------------------------------------- |
| `backend/`               | Go server: API, public pages, analytics ingest, billing, MCP     |
| `frontend/`              | React + TypeScript app (Vite, static SPA)                        |
| `docs/`                  | Product brief, local setup, deploy guide, dated audits           |
| `CLAUDE.md`, `AGENTS.md` | Rules for coding agents and contributors                         |
| `ROADMAP.md`             | What is built, what is next, what the owner still has to decide  |

Each feature has its own `README.md` next to its code, under
`backend/internal/<feature>/` and `frontend/src/client/features/<feature>/`.

## Development

- Run it locally: [`docs/LOCAL_DEVELOPMENT.md`](./docs/LOCAL_DEVELOPMENT.md)
- Server settings and routes: [`backend/README.md`](./backend/README.md)
- Deploy: [`docs/DEPLOY.md`](./docs/DEPLOY.md)

## Hosting

Production runs on Railway: one Go server that also serves the built React
app, with Postgres and Redis. The root `Dockerfile` builds both. Cloudflare
sits in front for DNS and forwards `support@uplumo.com`. Payments go through
Razorpay, transactional email through Loops, SEO data through DataForSEO on
Uplumo's own account (customers never bring their own keys).

Secrets live in Railway variables, never in this repository.
