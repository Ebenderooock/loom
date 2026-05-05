# syntax=docker/dockerfile:1.7

# ---------- frontend build ----------
FROM node:20-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-audit --no-fund || npm install --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---------- backend build ----------
FROM golang:1.24-alpine AS build
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./web/dist
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -tags embed \
    -ldflags "-s -w \
      -X github.com/loomctl/loom/internal/buildinfo.Version=${VERSION} \
      -X github.com/loomctl/loom/internal/buildinfo.Commit=${COMMIT} \
      -X github.com/loomctl/loom/internal/buildinfo.Date=${DATE}" \
    -o /out/loom ./cmd/loom

# ---------- runtime ----------
FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source="https://github.com/loomctl/loom"
LABEL org.opencontainers.image.licenses="AGPL-3.0-only"
LABEL org.opencontainers.image.title="Loom"
LABEL org.opencontainers.image.description="Unified media automation — Radarr/Sonarr/Prowlarr replacement"

USER nonroot:nonroot
WORKDIR /
COPY --from=build /out/loom /usr/local/bin/loom

EXPOSE 8989
VOLUME ["/config", "/media"]

ENV LOOM_HTTP_ADDR=:8989 \
    LOOM_CONFIG_DIR=/config \
    LOOM_DATA_DIR=/config

ENTRYPOINT ["/usr/local/bin/loom"]
CMD ["serve"]
