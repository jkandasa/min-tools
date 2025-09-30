# min-tools

Collection of tools for MinIO administration and monitoring.

## Tools

- **[prometheus/](prometheus/)** - MinIO bucket summary tool for Prometheus metrics
  - Parses MinIO Prometheus metrics
  - Generates bucket-level statistics
  - Tracks object versioning distribution

- **[stats/](stats/)** - MinIO cluster stats analyzer
  - Processes cluster diagnostic data
  - Shows server and disk status
  - Displays pool configurations

- **[generate-s3-data/](generate-s3-data/)** - S3 data generation tool for testing and auditing
  - Generates random S3 operations (READ, WRITE, OVERWRITE, DELETE, PREFIX DELETE, MULTIPART UPLOAD)
  - Supports authentication via access keys or MC aliases
  - Configurable operation frequency and duration
  - Real-time operation status display and statistics tracking

## Installation

### Pre-built Binaries

Download the latest pre-built binaries from the [GitHub Releases](https://github.com/jkandasa/min-tools/releases/latest):

- **Linux**: `*-linux-amd64` or `*-linux-arm64`
- **macOS**: `*-darwin-amd64` or `*-darwin-arm64`
- **Windows**: `*-windows-amd64.exe`

Make the binary executable and run:
```bash
chmod +x <binary-name>
./<binary-name>
```
