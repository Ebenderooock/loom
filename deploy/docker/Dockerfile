# syntax=docker/dockerfile:1.7

# ---------- frontend build ----------
FROM node:22-alpine AS web
RUN corepack enable && corepack prepare pnpm@9 --activate
WORKDIR /web
COPY web/package.json web/pnpm-lock.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm run build

# ---------- backend build ----------
FROM golang:1.26 AS build
WORKDIR /src
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates && rm -rf /var/lib/apt/lists/*
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./web/dist
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE
ARG TVDB_APIKEY=""
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -tags embed \
    -ldflags "-s -w \
      -X github.com/ebenderooock/loom/internal/buildinfo.Version=${VERSION} \
      -X github.com/ebenderooock/loom/internal/buildinfo.Commit=${COMMIT} \
      -X github.com/ebenderooock/loom/internal/buildinfo.Date=${DATE} \
      -X github.com/ebenderooock/loom/cmd/loom.defaultTVDBKey=${TVDB_APIKEY}" \
    -o /out/loom ./cmd/loom

# ---------- runtime ----------
FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/ebenderooock/loom"
LABEL org.opencontainers.image.licenses="AGPL-3.0-only"
LABEL org.opencontainers.image.title="Loom"
LABEL org.opencontainers.image.description="Unified media automation — Radarr/Sonarr/Prowlarr replacement"

USER nonroot:nonroot
WORKDIR /
COPY --from=build /out/loom /usr/local/bin/loom

EXPOSE 8989
VOLUME ["/config", "/media"]

ENV LOOM_CONFIG_DIR=/config \
    LOOM_DATA_DIR=/config

ENTRYPOINT ["/usr/local/bin/loom"]
CMD ["serve"]
