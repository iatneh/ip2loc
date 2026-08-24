# ip2loc

Lightweight MaxMind mmdb-backed IP geolocation service, written in Go.

## Endpoints

| Method | Path        | Description                                            |
|--------|-------------|--------------------------------------------------------|
| GET    | `/`         | Return the calling client's IP                         |
| GET    | `/ip2loc`   | `?ip=<addr>&lang=<locale>` → full geo + ASN JSON       |
| GET    | `/healthz`  | Liveness probe (always 200 while process is up)        |
| GET    | `/readyz`   | Readiness probe (200 only when city db is loaded)      |

### Example

```bash
$ curl 'http://localhost:8080/ip2loc?ip=183.11.242.230&lang=zh-CN'
{
  "code": 0,
  "message": "success",
  "data": {
    "ip": "183.11.242.230",
    "countryCode": "CN",
    "countryName": "中国",
    "regionCode": "GD",
    "regionName": "广东",
    "cityName": "深圳",
    "timeZone": "Asia/Shanghai",
    "asn": 4134,
    "asOrg": "Chinanet",
    ...
  }
}
```

## Configuration

`configs/app.yaml` is the default. Every key can be overridden by env vars with
prefix `IP2LOC_` and underscores for dots/dashes:

| YAML key                        | Env var                          |
|---------------------------------|----------------------------------|
| `http.port`                     | `IP2LOC_HTTP_PORT`               |
| `http.address`                  | `IP2LOC_HTTP_ADDRESS`            |
| `log.level`                     | `IP2LOC_LOG_LEVEL`               |
| `geoip.db-dir`                  | `IP2LOC_GEOIP_DB_DIR`            |
| `updater.cron`                  | `IP2LOC_UPDATER_CRON`            |
| `updater.city-url`              | `IP2LOC_UPDATER_CITY_URL`        |

See [`configs/app.yaml`](configs/app.yaml) for the full schema.

## Project layout

```
cmd/ip2loc/         entry point
internal/
  config/           YAML + env configuration
  geoip/            mmdb reader, atomic reload, mtime watcher
  iputil/           IP parsing, client IP extraction
  logger/           slog setup
  server/           HTTP handlers, middleware
  updater/          cron-driven mmdb downloader + atomic swap
configs/            default YAML
```

## Build & run

```bash
make build         # → ./build/ip2loc
./build/ip2loc -config ./configs/app.yaml

# or
make run

# docker
make docker
docker compose up
```

## What changed vs. the original

The original `iatneh/ip2loc` is a single-package Gin app. This rewrite keeps the
public API identical but reorganises around three concerns:

1. **Layered packages with explicit interfaces.** `updater` depends on a
   `Reloader` interface, not the concrete `geoip.Reader`, so tests can stub it.
2. **No global state.** `config.Config`, `*geoip.Reader` and `*updater.Updater`
   are constructed in `main` and passed by value; the package-level `appConf`
   is gone.
3. **Atomics, not locks, on the hot path.** The geoip reader swaps two
   `atomic.Pointer`s. Old readers close on a goroutine so an in-flight query
   never faults a half-mmap'd file.
4. **Hot reload by mtime polling.** A 30s watcher detects file changes and
   reloads in place — no SIGUSR1, no restart.
5. **Lighter deps.** Replaced `gin`, `gin-contrib/gzip`, `ginrus`, `logrus`,
   `viper`+`locafero` config glue, `robfig/cron v1`, `go-playground/validator`
   and the i18n translators with `net/http` (stdlib), `slog` (stdlib), `viper`
   for config, and `robfig/cron/v3`. Validator translations are gone — the API
   surface is too small to justify them.

## Tests

```bash
make test          # unit tests with -race
make test-cover    # + coverage report
```

End-to-end smoke (requires `./data/GeoLite2-City.mmdb`):

```bash
go run ./cmd/e2e_test
```
