# Build the React app, build the Go server, ship both in one small image.
FROM node:22 AS web
ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
RUN corepack enable && corepack prepare pnpm@10.30.1 --activate
WORKDIR /app
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY index.html vite.config.ts tsconfig.json components.json ./
COPY src ./src
RUN pnpm exec vite build

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
