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
  "msg": "success",
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

The service **does not read any configuration file**. All knobs are baked into
the binary as defaults, and overridden by environment variables with prefix
`IP2LOC_` and underscores for dashes.

Run `./build/ip2loc -print-defaults` to see every default dumped at runtime.

| Env var                          | Default                              | Notes |
|----------------------------------|--------------------------------------|-------|
| `IP2LOC_HTTP_ADDRESS`            | `0.0.0.0`                            | listen address |
| `IP2LOC_HTTP_PORT`               | `8080`                               | listen port |
| `IP2LOC_HTTP_READ_TIMEOUT`       | `10s`                                | Go duration syntax (`10s`, `500ms`) |
| `IP2LOC_HTTP_WRITE_TIMEOUT`      | `10s`                                | Go duration syntax |
| `IP2LOC_HTTP_IDLE_TIMEOUT`       | `60s`                                | Go duration syntax |
| `IP2LOC_LOG_LEVEL`               | `info`                               | `debug` / `info` / `warn` / `error` |
| `IP2LOC_LOG_FORMAT`              | `text`                               | `text` or `json` |
| `IP2LOC_LOG_OUTPUT`              | `stdout`                             | stream name (`stdout`/`stderr`) or file path |
| `IP2LOC_GEOIP_DB_DIR`            | `./data`                             | directory holding the mmdb files |
| `IP2LOC_GEOIP_CITY_FILENAME`     | `GeoLite2-City.mmdb`                 | file name inside `db-dir` |
| `IP2LOC_GEOIP_ASN_FILENAME`      | `GeoLite2-ASN.mmdb`                  | file name inside `db-dir` |
| `IP2LOC_GEOIP_DEFAULT_LANG`      | `en`                                 | fallback for localized names |
| `IP2LOC_UPDATER_ENABLED`         | `false`                              | flip to `true` to enable scheduled downloads |
| `IP2LOC_UPDATER_RUN_ON_START`    | `true`                               | run one download+reload cycle right after process boot |
| `IP2LOC_UPDATER_CRON`            | `0 0 */2 * * *`                      | cron-with-seconds; every 2 hours |
| `IP2LOC_UPDATER_DOWNLOAD_TIMEOUT`| `120s`                               | per-request timeout |
| `IP2LOC_UPDATER_CITY_URL`        | `https://git.io/GeoLite2-City.mmdb`  | git.io alias → P3TERX mirror |
| `IP2LOC_UPDATER_ASN_URL`         | `https://git.io/GeoLite2-ASN.mmdb`   | git.io alias → P3TERX mirror |
| `IP2LOC_UPDATER_HEADERS`         | (empty)                              | comma-separated `Name: value` pairs |
| `IP2LOC_DEFAULTS_ALLOW_PRIVATE_IP`| `true`                              | `true`/`false`/`1`/`0`/`yes`/`no` |

**Type-coercion rules for env values:**

| Field type             | Accepted env syntax                              |
|------------------------|--------------------------------------------------|
| `int`                  | decimal integer, e.g. `8080`                     |
| `time.Duration`        | Go duration string, e.g. `10s`, `500ms`, `2m`    |
| `bool`                 | `1`/`0`/`t`/`f`/`true`/`false`/`yes`/`no`/`on`/`off` (case-insensitive) |
| `[]string`             | comma-separated; whitespace trimmed per element  |
| other                  | exact string match                               |

### Enabling scheduled downloads

The updater is **off by default** — even though the `city-url` and `asn-url`
defaults are baked in, no download happens unless you also opt in:

```bash
IP2LOC_UPDATER_ENABLED=true
# these two already have defaults; override only if pointing at a private mirror
IP2LOC_UPDATER_CITY_URL=https://your-mirror/GeoLite2-City.mmdb
IP2LOC_UPDATER_ASN_URL=https://your-mirror/GeoLite2-ASN.mmdb
IP2LOC_UPDATER_CRON="0 0 */2 * * *"
```

Files are downloaded into `IP2LOC_GEOIP_DB_DIR` (default `./data`) under the
configured `city-filename` / `asn-filename`. The new file is md5-checked
against the existing one: identical → no rename (so the file-watcher mtime
polling stays quiet); different → atomic swap via `rename(2)`.

**Run-on-start is on by default.** As soon as `IP2LOC_UPDATER_ENABLED=true`
and the cron is scheduled, a single download+reload cycle fires in a
goroutine so a freshly started container reaches `/readyz` without waiting
up to one cron interval. Set `IP2LOC_UPDATER_RUN_ON_START=false` to keep the
legacy "first download only on the next cron tick" behaviour.

The service also hot-reloads the mmdb files via a 30s mtime watcher, so
dropping a new `.mmdb` into the db-dir by hand picks it up without a restart.

### Running in Docker

The container image (`iatneh1900/ip2loc`) listens on `:8080` and exposes
`/opt/data` as a volume for the mmdb cache. The CI pipeline downloads the
latest `GeoLite2-City.mmdb` and `GeoLite2-ASN.mmdb` from the P3TERX mirror
into `./db-cache/` and the Dockerfile `COPY`s them into `/opt/data`, so a
freshly pulled image is immediately ready to serve lookups — no
first-start download required.

You can still mount your own directory over `/opt/data` to override the
baked-in copies, and you should still enable the in-container updater if
you want the data to refresh automatically between image updates.

```bash
docker run --rm -p 8080:8080 \
  -v $(pwd)/data:/opt/data \
  -e IP2LOC_UPDATER_ENABLED=true \
  iatneh1900/ip2loc:latest
```

`docker compose up` (from `docker-compose.yaml`) maps host port `8080` to the
container.

#### Pointing the baked-in copy at a private mirror

The CI step reads two optional repo-level Variables (Settings → Secrets and
variables → Actions → Variables):

| Variable         | Default                              |
|------------------|--------------------------------------|
| `CITY_MMDB_URL`  | `https://git.io/GeoLite2-City.mmdb`  |
| `ASN_MMDB_URL`   | `https://git.io/GeoLite2-ASN.mmdb`   |

Set them to point at your private MaxMind mirror; the next CI run bakes
those copies into the image.

For local builds, drop your mmdb files into `./db-cache/` before invoking
`docker build`:

```bash
cp /path/to/GeoLite2-City.mmdb db-cache/
cp /path/to/GeoLite2-ASN.mmdb  db-cache/
docker build -t ip2loc:custom .
```

If `./db-cache/` only contains the placeholder `.gitkeep`, the image
ships with an empty `/opt/data` and the in-container updater (when
enabled) fills it on first start.