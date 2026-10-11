# Local development

## Prerequisites

- Go (version in `backend/go.mod`)
- Node.js 22+ with [Corepack](https://nodejs.org/api/corepack.html)
- Postgres and Redis (for example `docker run` the official images)

## Run

```sh
cp .env.example .env     # fill in BETTER_AUTH_SECRET and the database URLs
corepack enable
pnpm install --frozen-lockfile

# Terminal 1: the Go server (applies migrations on start)
cd backend && set -a && . ../.env && set +a && go run ./cmd/server

# Terminal 2: the React app with hot reload; it proxies /api, /s, /r, /mcp
# and /webhooks to the Go server on :8080
pnpm dev
```

Open <http://localhost:3001>. To serve the production build from Go instead,
run `pnpm exec vite build` and start the server with `STATIC_DIR=../dist`.

## Checks

```sh
pnpm ci:check && pnpm test           # frontend
cd backend && go vet ./... && go test -race ./...   # needs TEST_DATABASE_URL
```

## Report share images

The share page uses the static `/social-card.png` as its preview image.
Per-report images are not generated yet.
