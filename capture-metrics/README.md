# MinIO Metrics

Collect MinIO cluster metrics at regular intervals and store them in rotated, compressed CSV files.

## Features

- Periodic metrics collection (configurable interval: 30s, 1m, 5m, etc.)
- Automatic file rotation when size limit reached (default: 100 MiB)
- Gzip compression for rotated files (80-90% space savings)
- Configurable file retention policy
- Per-metric CSV files with full Prometheus label support
- Graceful shutdown with proper file handling

## Quick Start

```bash
# Build
make build

# Run
./minio-metrics -endpoint "https://minio.example.com" -token "your-jwt-token"

# Check version
./minio-metrics -version
```

## Installation

**Prerequisites:** Go 1.21 or higher

```bash
cd minio-metrics
make build
# Or: go build -ldflags "-s -w" -o minio-metrics
```

## Usage

### Required Flags

- `-endpoint <url>` - MinIO endpoint URL
- `-token <jwt>` - JWT authentication token (obtain via `mc admin prometheus generate <alias>`)

### Optional Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-interval` | `1m` | Collection interval (e.g., 30s, 5m, 1h) |
| `-metrics` | `minio_node_drive_total_bytes,minio_node_drive_used_bytes` | Comma-separated metrics list |
| `-output-dir` | `./metrics` | Directory for metric files |
| `-max-file-size` | `100` | File size in MiB before rotation |
| `-retain-count` | `0` | Keep N recent rotated files (0 = keep all) |
| `-debug` | `false` | Enable verbose logging |

### Example

```bash
./minio-metrics \
  -endpoint "https://minio.example.com" \
  -token "eyJhbGc..." \
  -interval 5m \
  -max-file-size 50 \
  -retain-count 20 \
  -output-dir "/var/log/minio-metrics"
```

## Output Format

**Directory Structure:**
```
metrics/
├── minio_node_drive_total_bytes.csv          # Active file
├── minio_node_drive_total_bytes_*.csv.gz     # Rotated (compressed)
└── minio_node_drive_used_bytes.csv
```

**CSV Format:**
```csv
timestamp,metric,labels,value
2025-12-05T15:47:21Z,minio_node_drive_total_bytes,"drive=/data1,server=192.168.1.10:9000",1000000000000
2025-12-05T15:48:21Z,minio_s3_requests_total,"api=putobject,server=play.min.io:9000",42
```

**Working with compressed files:**
```bash
zcat file.csv.gz | head                # View without decompressing
gunzip file.csv.gz                     # Decompress
gunzip metrics/*.gz                    # Decompress all
```

## Common Metrics

**Node:** `minio_node_drive_total_bytes`, `minio_node_drive_used_bytes`, `minio_node_drive_free_bytes`
**Cluster:** `minio_cluster_capacity_usable_total_bytes`, `minio_cluster_capacity_usable_free_bytes`
**S3 API:** `minio_s3_requests_total`, `minio_s3_errors_total`, `minio_s3_requests_current`

Refer to MinIO documentation for a complete list.

## Running as a Service

**Systemd Service** (`/etc/systemd/system/minio-metrics.service`):

```ini
[Unit]
Description=MinIO Metrics Collector
After=network.target

[Service]
Type=simple
User=minio
ExecStart=/usr/local/bin/minio-metrics -endpoint "https://minio.example.com" -token "your-jwt-token" -output-dir "/var/log/minio-metrics"
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable minio-metrics
sudo systemctl start minio-metrics
```

## Troubleshooting

**Connection Issues:**
- Collector accepts self-signed certificates by default
- Verify JWT token validity (tokens expire)
- Check endpoint URL: `https://your-minio/minio/v2/metrics/cluster`

**No Metrics Collected:**
- Verify metric names are correct
- Ensure JWT token has proper permissions
- Use `-debug` flag for detailed logs

**File Permissions:**
```bash
mkdir -p /path/to/metrics && chmod 755 /path/to/metrics
```

## Building

```bash
make build           # Build for current platform
make build-all       # Build for all platforms
make clean           # Clean build artifacts
```

**Build flags include:**
- Version information from git tags
- Stripped debug symbols for smaller binaries
- Commit hash and build date

## License

Provided as-is for monitoring MinIO clusters.
