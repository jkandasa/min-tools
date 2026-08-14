# min-collector

Collect server logs, audit events and metrics from a MinIO cluster and store
them in rotated, compressed files.

One process handles all three streams:

| Stream  | How it is collected                        | Default state | Format |
|---------|--------------------------------------------|---------------|--------|
| logger  | MinIO pushes to the `/logger` webhook      | enabled       | NDJSON |
| audit   | MinIO pushes to the `/audit` webhook       | enabled       | NDJSON |
| metrics | min-collector scrapes the Prometheus endpoint | disabled   | CSV    |

Rotation, compression (zstd, gzip or xz) and retention apply to every stream.

## Quick start

```bash
make build

# Receive logger and audit webhooks on :8080
./min-collector --output-dir /var/log/min-collector
```

The startup banner prints the exact `mc` commands to point MinIO at the
collector:

```bash
mc admin config set myminio logger_webhook:min-collector endpoint="http://collector-host:8080/logger"
mc admin config set myminio audit_webhook:min-collector  endpoint="http://collector-host:8080/audit"
mc admin service restart myminio
```

Add the metrics scrape when it is needed:

```bash
./min-collector \
  --metrics \
  --metrics-endpoint "https://minio.example.com" \
  --metrics-token "$(mc admin prometheus generate myminio --json | jq -r .token)"
```

## Installation

Prerequisites: Go 1.24 or higher.

```bash
make build              # builds ./min-collector
make install            # copies it to /usr/local/bin
make build-all          # linux, darwin and windows binaries
```

## Configuration

Everything can be set on the command line, in a YAML file, or, for the two
tokens, in the environment. A value is resolved in this order, each step
overwriting the previous one:

```
defaults  ->  --config file  ->  flags  ->  environment (tokens only)
```

Only keys present in the YAML file are applied, the rest keep their default.
Only flags actually passed are applied, so an untouched flag never overwrites
the file.

Write a starting point and edit it:

```bash
./min-collector --print-config > /etc/min-collector.yaml
./min-collector --config /etc/min-collector.yaml
```

```yaml
# /etc/min-collector.yaml
listen: ":8080"
auth_token: ""          # better supplied by MIN_COLLECTOR_AUTH_TOKEN

logger:
  enabled: true
  path: /logger

audit:
  enabled: true
  path: /audit

metrics:
  enabled: false
  endpoint: ""          # https://minio.example.com
  token: ""             # better supplied by MIN_COLLECTOR_METRICS_TOKEN
  path: /minio/v2/metrics/cluster
  interval: 1m
  list:                 # [all] collects every metric
    - minio_node_drive_total_bytes
    - minio_node_drive_used_bytes
  insecure: true

storage:
  output_dir: ./collected
  max_file_size: 100    # MiB
  retain_count: 0       # 0 keeps all rotated files
  compress: zstd        # zstd, gzip, xz or none
  flush_interval: 5s

log:
  level: info           # debug, info, warn or error
  format: console       # console or json
  stats_interval: 5m
```

An unknown key is an error rather than being silently ignored, so a typo shows
up at startup. `--print-config` redacts the tokens, it is safe to commit.

### Environment variables

Only the two secrets are read from the environment, and they win over both the
file and the flags so a service manager can inject them without putting them in
the command line or on disk.

| Variable | Sets |
|----------|------|
| `MIN_COLLECTOR_AUTH_TOKEN` | the webhook authorization token |
| `MIN_COLLECTOR_METRICS_TOKEN` | the JWT for the metrics scrape |

## Usage

Run `./min-collector --help` for the grouped flag list. Nothing is required, the
defaults collect logger and audit on `:8080` into `./collected`.

### Server

| Flag | Default | Description |
|------|---------|-------------|
| `--listen` | `:8080` | Address the webhook server listens on. `9500` is accepted as shorthand for `:9500` |
| `--auth-token` | empty | Token MinIO must send in the `Authorization` header. Empty disables the check |

### Logger and audit

| Flag | Default | Description |
|------|---------|-------------|
| `--logger` | `true` | Collect server logs |
| `--logger-path` | `/logger` | Path that receives the logger webhook |
| `--audit` | `true` | Collect audit events |
| `--audit-path` | `/audit` | Path that receives the audit webhook |

Disable one with `--logger=false` or `--audit=false`.

### Metrics

Metrics are **off by default**, `--metrics` turns the scrape on.

| Flag | Default | Description |
|------|---------|-------------|
| `--metrics` | `false` | Enable the Prometheus scrape |
| `--metrics-endpoint` | empty | MinIO server URL, required with `--metrics` |
| `--metrics-token` | empty | JWT from `mc admin prometheus generate <alias>`, required with `--metrics` |
| `--metrics-interval` | `1m` | Scrape interval, for example `30s`, `5m`, `1h` |
| `--metrics-list` | `minio_node_drive_total_bytes,minio_node_drive_used_bytes` | Metrics to keep, or `all` |
| `--metrics-path` | `/minio/v2/metrics/cluster` | Metrics path on the MinIO server |
| `--insecure` | `true` | Skip TLS verification, MinIO clusters often use self signed certificates |

With an explicit list each metric gets its own file. With `--metrics-list all`
every metric goes into a single `all_metrics.csv`, so the directory does not
fill up with hundreds of files.

Other metrics paths worth using: `/minio/v2/metrics/node`,
`/minio/v2/metrics/bucket`, `/minio/v2/metrics/resource`.

### Storage, rotation and compression

These apply to every stream.

| Flag | Default | Description |
|------|---------|-------------|
| `--output-dir` | `./collected` | Root directory of the collected data |
| `--max-file-size` | `100` | Size in MiB at which a file is rotated |
| `--retain-count` | `0` | Rotated files kept per stream, 0 keeps all |
| `--compress` | `zstd` | Compression of rotated files: `zstd`, `gzip`, `xz` or `none` |
| `--flush-interval` | `5s` | How often buffered records reach the disk |

#### Choosing a compression

| Value | Extension | Read with | When to use it |
|-------|-----------|-----------|----------------|
| `zstd` | `.zst` | `zstdcat` | Default. Close to xz in size at gzip like speed |
| `gzip` | `.gz` | `zcat` | Maximum portability, every host can read it |
| `xz` | `.xz` | `xzcat` | Smallest files, for long term archives. Slowest and most CPU hungry |
| `none` | | `cat` | Keep rotated files as plain text |

Compression runs in the background, one rotated file at a time, so it never
blocks incoming events.

### Misc

| Flag | Default | Description |
|------|---------|-------------|
| `--stats-interval` | `5m` | How often a collection summary is logged, `0` disables it |
| `--log-level` | `info` | `debug`, `info`, `warn` or `error` |
| `--log-format` | `console` | `console` or `json` |

| `--debug` | | Shortcut for `--log-level debug` |
| `--config` | | Path to a YAML configuration file |
| `--print-config` | | Print the effective configuration as YAML and exit |
| `--version` | | Print version information |

Logs go to stderr and carry the source file and line they came from, so a
message can be traced back to the code that emitted it:

```
2026-08-14T19:22:08.555+0530  INFO   app/app.go:194         audit webhook listening on :8080/audit -> /var/log/min-collector/audit/audit.ndjson
2026-08-14T19:22:09.563+0530  DEBUG  webhook/receiver.go:109  audit: stored 1 event(s) from 10.0.3.15:58290
```

```json
{"level":"info","time":"2026-08-14T19:22:13.715+0530","caller":"app/app.go:194","msg":"audit webhook listening on :8080/audit -> /var/log/min-collector/audit/audit.ndjson"}
```

Both `--flag` and `-flag` are accepted, the double dash form is used throughout
this document.

## Output layout

```
collected/
├── logger/
│   ├── logger.ndjson                         # active file
│   └── logger_20260814_120000.ndjson.zst     # rotated, compressed
├── audit/
│   ├── audit.ndjson
│   └── audit_20260814_120000.ndjson.zst
└── metrics/
    ├── minio_node_drive_total_bytes.csv
    └── minio_node_drive_total_bytes_20260814_120000.csv.zst
```

Logger and audit events are stored one JSON document per line, exactly as MinIO
sent them:

```json
{"version":"1","time":"2026-08-14T12:00:01Z","api":{"name":"PutObject"},"remotehost":"10.0.0.5"}
```

Metrics are stored as CSV with a header on every file:

```csv
timestamp,metric,labels,value
2026-08-14T12:00:00Z,minio_node_drive_total_bytes,"drive=/data1,server=192.168.1.10:9000",1000000000000
```

Working with the collected data:

```bash
jq -r '.api.name' collected/audit/audit.ndjson | sort | uniq -c   # audit summary
zstdcat collected/audit/audit_*.ndjson.zst | jq -r 'select(.api.statusCode >= 400)'
zstdcat collected/metrics/*.csv.zst | head
```

Use `zcat` or `xzcat` instead when the collector runs with `--compress gzip` or
`--compress xz`.

## Connecting MinIO

```bash
# Server logs
mc admin config set myminio logger_webhook:min-collector \
  endpoint="http://collector-host:8080/logger"

# Audit events, batch_size is optional and reduces the request rate
mc admin config set myminio audit_webhook:min-collector \
  endpoint="http://collector-host:8080/audit" \
  batch_size=100 queue_size=100000

mc admin service restart myminio
```

With `--auth-token` set, add the same value to both targets:

```bash
mc admin config set myminio audit_webhook:min-collector \
  endpoint="http://collector-host:8080/audit" auth_token="my-secret"
```

Check that events are arriving:

```bash
curl -s http://collector-host:8080/health | jq
```

```json
{
  "status": "ok",
  "streams": {
    "audit":  {"events": 1832, "rejected": 0, "errors": 0, "file": "/var/log/min-collector/audit/audit.ndjson"},
    "logger": {"events": 12,   "rejected": 0, "errors": 0, "file": "/var/log/min-collector/logger/logger.ndjson"}
  }
}
```

## Running as a service

`/etc/systemd/system/min-collector.service`:

```ini
[Unit]
Description=MinIO log, audit and metrics collector
After=network.target

[Service]
Type=simple
User=minio
# Keep the secrets out of the command line and the config file
EnvironmentFile=/etc/min-collector.env
ExecStart=/usr/local/bin/min-collector -config /etc/min-collector.yaml
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

`/etc/min-collector.env`, readable only by the service user:

```bash
MIN_COLLECTOR_AUTH_TOKEN=my-secret
MIN_COLLECTOR_METRICS_TOKEN=eyJhbGc...
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now min-collector
```

`SIGTERM` and `SIGINT` stop the collector, flush the open files and finish any
compression in progress.

## Project layout

```
cmd/min-collector/     entry point
internal/config/       flags, environment and validation
internal/webhook/      logger and audit HTTP receivers
internal/metrics/      Prometheus scraper and parser
internal/rotate/       rotating, compressing file writer
internal/compress/     zstd, gzip and xz encoders
internal/logx/         zap based logging
internal/app/          wiring and lifecycle
```

```bash
make test    # unit tests
make vet
make lint    # requires golangci-lint
```

## Troubleshooting

**No events arriving.** Confirm MinIO can reach the collector, the webhook is
`on` in `mc admin config get <alias> audit_webhook`, and that MinIO was
restarted after the configuration change. `/health` shows the per stream
counters, `--debug` logs every request.

**401 in the collector log.** The `auth_token` on the MinIO target does not
match `--auth-token`.

**Metrics scrape fails with 403.** The JWT expired, regenerate it with
`mc admin prometheus generate <alias>`.

**Metrics scrape fails with a TLS error.** Pass `http://` explicitly if the
server is plain HTTP, an endpoint without a scheme is treated as HTTPS.

**No matching samples.** The names in `--metrics-list` do not exist on that
endpoint. Try `--metrics-list all`, or a different `--metrics-path`.

**Files are not rotating.** Rotation happens on write, once the active file
passes `--max-file-size`. A single record larger than the limit is still written
whole rather than being split.

## License

Provided as-is for monitoring MinIO clusters.
