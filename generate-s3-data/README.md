# Generate S3 Data

A tool that generates S3 data by performing random operations (read, write, overwrite, delete, prefix delete, multipart upload) on a MinIO server. This tool is designed for testing and audit purposes.

## Features

- Performs random operations: READ, WRITE, OVERWRITE, DELETE, PREFIX DELETE, MULTIPART UPLOAD
- Connects using MinIO access/secret keys or MC aliases
- Configurable operation frequency and duration  
- Real-time operation status display
- Statistics tracking and reporting
- Can run for a specified duration or indefinitely

## Installation

```bash
go build -o generate-s3-data
```

## Usage

### Using Access Key and Secret Key

```bash
./generate-s3-data \
  --endpoint localhost:9000 \
  --access-key YOUR_ACCESS_KEY \
  --secret-key YOUR_SECRET_KEY \
  --buckets test-bucket \
  --duration 5m \
  --delay 2s
```

### Using MC Alias (MC Config File)

First, configure your MC alias using the MinIO Client:

```bash
mc alias set myalias http://localhost:9000 your_access_key your_secret_key
```

This creates an entry in `~/.mc/config.json`. Then run:

```bash
./generate-s3-data --alias myalias --duration 10m
```

### Command Line Options

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--endpoint` | `-e` | MinIO server endpoint | `localhost:9000` |
| `--access-key` | `-a` | MinIO access key | |
| `--secret-key` | `-s` | MinIO secret key | |
| `--buckets` | `-b` | MinIO bucket names (comma-separated) | `test-bucket` |
| `--ssl` | | Use SSL connection | `false` |
| `--alias` | | Use MC alias instead of keys | |
| `--duration` | `-d` | Duration to run (0 for infinite) | `0` |
| `--delay` | | Delay between operations | `1s` |
| `--prefix` | `-p` | Object name prefix | `test-object` |

## Examples

### Run for 30 minutes with 500ms delay between operations

```bash
./generate-s3-data \
  --endpoint minio.example.com:9000 \
  --access-key minioadmin \
  --secret-key minioadmin \
  --buckets audit-test \
  --duration 30m \
  --delay 500ms \
  --ssl
```

### Run indefinitely (until Ctrl+C)

```bash
./generate-s3-data \
  --endpoint localhost:9000 \
  --access-key minioadmin \
  --secret-key minioadmin \
  --buckets stress-test \
  --duration 0
```

### Use custom object prefix

```bash
./generate-s3-data \
  --endpoint localhost:9000 \
  --access-key minioadmin \
  --secret-key minioadmin \
  --buckets test \
  --prefix audit-object \
  --duration 1h
```

### Multiple Buckets Support

```bash
./generate-s3-data \
  --endpoint localhost:9000 \
  --access-key minioadmin \
  --secret-key minioadmin \
  --buckets "bucket1,bucket2,bucket3" \
  --duration 1h
```

When multiple buckets are specified:
- Write operations randomly select a target bucket
- Read/delete operations search across all buckets
- All buckets are automatically created if they don't exist
- Operation logs show which bucket was used (e.g., `bucket2/object-name`)

## Object Naming

Objects are created with **random prefixes** and human-readable timestamps including milliseconds:

**Format:** `{random-prefix}/{base-prefix}-{YYYY-MM-DDTHH-MM-SS-mmm}-{random}[-m]`

**Random Prefix Structure:** Objects are distributed across random directory-like paths such as:
- `data/2025/09/30/`
- `logs/batch-001/monthly/`
- `backup/user-002/q3/prod/`
- `temp/session-a/weekly/`

**Examples:** 
- `logs/2025/09/test-object-2025-09-30T18-59-33-123-4567` (regular: 100B-5KB)
- `data/batch-001/daily/test-object-2025-09-30T18-59-33-456-7890-m` (multipart: 70MB)

This ensures objects are:
- **Distributed across prefixes**: Simulates real-world S3 usage patterns
- **Easily sortable by creation time**: Within each prefix path
- **Uniquely identifiable**: Even across different prefixes
- **Human-readable for debugging**: Clear timestamps and logical paths
- **Distinguishable by type**: Regular vs multipart uploads (`-m` suffix)

## Output

The tool provides real-time feedback on operations:

```
Starting MinIO audit sidecar...
Endpoint: localhost:9000
Bucket: test-bucket
Duration: 5m0s (0 = infinite)
Operation Delay: 1s
Press Ctrl+C to stop
==================================================
[SUCCESS] WRITE: test-object-1727123456-1234 (1024 bytes)
[SUCCESS] READ: test-object-1727123456-1234 (1024 bytes)  
[SUCCESS] OVERWRITE: test-object-1727123456-1234 (2048 bytes)
[SUCCESS] DELETE: test-object-1727123456-1234
[ERROR] Operation failed: read operation failed: The specified key does not exist

[STATS] Read=15, Write=12, Overwrite=8, Delete=10, PrefixDel=3, Multipart=2, Errors=2
```

## Operations

### WRITE
Creates a new object with random content (100-5120 bytes).

### READ  
Reads a randomly selected existing object. If no objects exist, creates one first.

### OVERWRITE
Overwrites a randomly selected existing object with new random content. If no objects exist, creates one first.

### DELETE
Deletes a randomly selected existing object. If no objects exist, creates one first then deletes it.

### PREFIX DELETE
Performs bulk deletion by removing all objects under a randomly selected prefix (directory path). Groups objects by their first 2 directory levels and deletes entire prefix contents. This simulates directory-level cleanup and bulk data lifecycle operations.

### MULTIPART UPLOAD
Creates large objects (70MB) using S3's multipart upload protocol with 5MB parts. Objects are identified with `-m` suffix for easy recognition.

## MC Alias Configuration

The tool reads MC aliases from `~/.mc/config.json`. This file is automatically created and managed by the MinIO Client (`mc`). 

To set up an alias:
```bash
mc alias set myalias https://minio.example.com:9000 ACCESS_KEY SECRET_KEY
```

The config file structure looks like:
```json
{
  "version": "10",
  "aliases": {
    "myalias": {
      "url": "https://minio.example.com:9000",
      "accessKey": "ACCESS_KEY",
      "secretKey": "SECRET_KEY",
      "api": "s3v4",
      "path": "auto"
    }
  }
}
```

## Requirements

- Go 1.24+
- Access to a MinIO server
- Valid MinIO credentials (access key/secret key or MC alias setup)
- MinIO Client (`mc`) for alias management (optional)

## Development

The project uses the following main dependencies:

- `github.com/minio/minio-go/v7` - MinIO Go SDK
- `github.com/spf13/cobra` - CLI framework

To run in development mode:

```bash
go run main.go --endpoint localhost:9000 --access-key minioadmin --secret-key minioadmin
```