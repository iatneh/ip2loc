# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS build
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
COPY configs/app.yaml /opt/configs/app.yaml

WORKDIR /opt
EXPOSE 8080
VOLUME ["/opt/data"]
ENTRYPOINT ["/opt/ip2loc"]
CMD ["-config", "/opt/configs/app.yaml"]
