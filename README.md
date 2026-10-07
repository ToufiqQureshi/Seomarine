# Seomarine

SEO and AI-search platform: keyword research, rank tracking, site audits,
backlinks, white-label client reports, and analytics that shows which AI
assistants (ChatGPT, Perplexity, Gemini, Claude, Copilot) send you visitors.

Proprietary software. See [`LICENSE`](./LICENSE).

## Where things live

| Path                         | What                                                     |
| ---------------------------- | -------------------------------------------------------- |
| `backend/`                   | Go server: API, analytics ingest, billing, landing pages |
| `src/client/`, `src/routes/` | React app (Vite)                                         |
| `src/server/`                | Legacy TypeScript backend, being ported to Go            |
| `docs/`                      | Setup and self-hosting guides                            |
| `CLAUDE.md`, `AGENTS.md`     | Rules for coding agents and contributors                 |
| `ROADMAP.md`                 | Feature status and priorities                            |

## Development

- Go backend: [`backend/README.md`](./backend/README.md)
- App: [`docs/LOCAL_DEVELOPMENT.md`](./docs/LOCAL_DEVELOPMENT.md)

SEO data comes from [DataForSEO](https://dataforseo.com/). Setup:
[`docs/DATAFORSEO_API_KEY.md`](./docs/DATAFORSEO_API_KEY.md).

## Hosting

Production runs on Railway (Go server, Postgres, Redis). Secrets live in
Railway variables, never in this repository.
