# syntax=docker/dockerfile:1.7

# ---- Stage 1: frontend build ----
FROM node:25-alpine AS frontend
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ---- Stage 2: backend build ----
FROM golang:1.25-alpine AS backend
WORKDIR /src
RUN apk add --no-cache git
# Allow overriding the Go module proxy at build time for environments where
# proxy.golang.org is slow or unreachable (e.g. CN networks).
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Frontend dist from stage 1 overrides the placeholder so go:embed picks it up.
# (vite outDir is `../web/dist` relative to /app, resolving to /web/dist)
COPY --from=frontend /web/dist ./web/dist
RUN CGO_ENABLED=0 go build \
      -trimpath \
      -ldflags="-s -w" \
      -o /out/portpass \
      ./cmd/server/

# ---- Stage 3: runtime ----
FROM alpine:3.20
RUN apk add --no-cache \
      ca-certificates \
      tzdata \
      iptables \
      ip6tables \
      nftables \
 && adduser -D -u 10001 portpass \
 && mkdir -p /data \
 && chown portpass:portpass /data

COPY --from=backend /out/portpass /usr/local/bin/portpass

ENV PORTPASS_LISTEN=":8080" \
    PORTPASS_DATA_DIR="/data" \
    PORTPASS_FIREWALL_DRIVER="iptables" \
    PORTPASS_AUTH_MODE="password" \
    TZ="UTC"

VOLUME ["/data"]
EXPOSE 8080

# NET_ADMIN is required for every supported firewall driver. Run the
# container with `--cap-add=NET_ADMIN --network=host` in production.
USER root
ENTRYPOINT ["/usr/local/bin/portpass"]
