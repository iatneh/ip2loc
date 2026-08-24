# syntax=docker/dockerfile:1
FROM golang:1.24.9-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
      -o /out/ip2loc ./cmd/ip2loc

FROM alpine:latest
RUN apk add --no-cache ca-certificates tzdata
LABEL org.opencontainers.image.source="https://github.com/iatneh/ip2loc"

COPY --from=build /out/ip2loc /opt/ip2loc

# Pre-populated mmdb cache from the CI workflow. The CI downloads the latest
# GeoLite2-City.mmdb / GeoLite2-ASN.mmdb into ./db-cache/ before invoking
# `docker build`, so a freshly pulled image is immediately ready to serve
# lookups — no first-start download required.
#
# `db-cache/.gitkeep` (filtered by .dockerignore) keeps the directory present
# in the build context for local builds; in that case /opt/data ships empty
# and the in-container updater (if enabled) fills it on first start.
COPY db-cache/ /opt/data/

WORKDIR /opt
EXPOSE 8080
VOLUME ["/opt/data"]
ENTRYPOINT ["/opt/ip2loc"]
