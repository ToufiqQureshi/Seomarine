# Build the React app, build the Go server, ship both in one small image.
FROM node:22 AS web
ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
RUN corepack enable && corepack prepare pnpm@10.30.1 --activate
WORKDIR /app
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml frontend/.npmrc ./
RUN pnpm install --frozen-lockfile
COPY frontend/index.html frontend/vite.config.ts frontend/tsconfig.json frontend/components.json ./
COPY frontend/src ./src
# The app lazy-loads e2e fixtures from tests/, so the bundler must see them.
COPY frontend/tests ./tests
# Vite bakes these into the bundle, so they are build arguments, not runtime
# variables.
ARG BYPASS_EMAIL_VERIFICATION=
ARG TURNSTILE_SITE_KEY=
ARG POSTHOG_PUBLIC_KEY=
ARG POSTHOG_HOST=
RUN BYPASS_EMAIL_VERIFICATION=$BYPASS_EMAIL_VERIFICATION     TURNSTILE_SITE_KEY=$TURNSTILE_SITE_KEY POSTHOG_PUBLIC_KEY=$POSTHOG_PUBLIC_KEY     POSTHOG_HOST=$POSTHOG_HOST pnpm exec vite build

FROM golang:1.27 AS server
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend ./
RUN CGO_ENABLED=0 go build -trimpath -o /out/server ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -o /out/erase-user ./cmd/erase-user

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/server /out/erase-user /
COPY --from=web /app/dist /dist
ENV STATIC_DIR=/dist
EXPOSE 8080
ENTRYPOINT ["/server"]
