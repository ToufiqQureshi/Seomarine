# Uplumo

SEO and AI-search platform: keyword research, rank tracking, site audits,
backlinks, white-label client reports, and analytics that shows which AI
assistants (ChatGPT, Perplexity, Gemini, Claude, Copilot) send you visitors.

Proprietary software. See [`LICENSE`](./LICENSE).

## Where things live

| Path                         | What                                                     |
| ---------------------------- | -------------------------------------------------------- |
| `backend/`                   | Go server: API, analytics ingest, billing, landing pages |
| `frontend/`                  | React + TypeScript app (Vite, static SPA)                |
| `docs/`                      | Setup guides                                             |
| `CLAUDE.md`, `AGENTS.md`     | Rules for coding agents and contributors                 |
| `ROADMAP.md`                 | Feature status and priorities                            |

## Development

- Go backend: [`backend/README.md`](./backend/README.md)
- App: [`docs/LOCAL_DEVELOPMENT.md`](./docs/LOCAL_DEVELOPMENT.md)

SEO data comes from [DataForSEO](https://dataforseo.com/) through Uplumo's
own account; customers never bring their own keys.

## Hosting

Production runs on Railway (one Go server that also serves the built React
app, plus Postgres and Redis). The root `Dockerfile` builds both. Secrets live in
Railway variables, never in this repository.
