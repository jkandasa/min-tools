# MinIO Cluster Stats Analyzer

A command-line tool for parsing and analyzing MinIO cluster information from JSON diagnostic files.

## Overview

This tool processes MinIO cluster diagnostic data (typically from `mc admin info` or subnet diagnostics) and presents it in a human-readable format, showing:
- Server status and statistics
- Disk/drive status per erasure set
- Pool configurations
- Overall cluster metrics

## Usage

```bash
go run main.go <filename> [domain-string]
```

### Parameters

- `filename` (required): Path to the JSON file containing MinIO cluster information
- `domain-string` (optional): Domain suffix to trim from endpoint names for cleaner output

### Examples

```bash
# Basic usage
go run main.go cluster-info.json

# With domain trimming
go run main.go cluster-info.json ".example.com"
```

## Input Format

The tool accepts JSON data in two formats:

1. **Direct cluster info format**: Standard output from `mc admin info --json`
2. **Subnet diagnostics format**: Data from MinIO subnet diagnostics with info nested under `"minio"` key

The tool automatically detects and handles both formats.

## Output

The tool displays:

### Per Pool Information
- **Server details**: Endpoint, state, version, memory stats, uptime
- **Erasure sets**: Disk status for each set showing:
  - Endpoint and drive path
  - Disk state (ok, offline, etc.)
  - Disk usage percentage and total space
  - Inode usage percentage
  - Metrics (if available): tokens, writes, deletes, waiting, timeouts, errors

### Overall Statistics
- Deployment ID
- Total sets and parity configuration
- Bucket, object, version, and delete marker counts
- Total storage usage
- Raw drive statistics

### Drive Status Summary
A summary map showing the count of drives in each state per pool.

## Building

```bash
go mod download
go build -o minio-stats main.go
```

## Data Structures

The tool processes the following key information:

- **Cluster Status**: Overall health and error states
- **Server Properties**: Individual server metrics including memory, ILM status, and uptime
- **Drive Status**: Per-drive information including:
  - Pool and set indices
  - Path and endpoint
  - Space usage (bytes and inodes)
  - Performance metrics (operations, errors, timeouts)

## Features

- Automatic format detection for different MinIO diagnostic outputs
- Domain trimming for cleaner endpoint display
- Natural sorting of endpoints for better readability
- Human-readable formatting of sizes and durations
- Detailed metrics display when available
- Pool and erasure set organization

## Future Enhancements

The code includes infrastructure for a future interactive TUI (Terminal User Interface) using `tview`, currently commented out in the `drawTable()` function.