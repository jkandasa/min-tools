# Generate S3 Data Tool - AI Assistant Prompt

## Tool Overview

You are assisting with the **`generate-s3-data`** tool, a comprehensive S3 data generator designed for MinIO testing, performance benchmarking, and audit log generation.

## Core Functionality

The tool performs **6 types of randomized S3 operations**:

1. **WRITE** - Creates small objects (100B-5KB) with random content
2. **READ** - Reads existing objects randomly from the bucket
3. **OVERWRITE** - Updates existing objects with new content
4. **DELETE** - Removes individual objects
5. **PREFIX DELETE** - Bulk deletes entire directory trees (all objects under a prefix)
6. **MULTIPART WRITE** - Creates large objects (70MB) using multipart upload protocol

## Key Features

### Random Prefix Distribution
- Objects are distributed across random directory-like paths
- Prefixes: `data/2025/09/`, `logs/batch-001/monthly/`, `temp/user-002/weekly/staging/`
- 2-4 levels deep with realistic naming patterns
- Prevents S3 hotspots and simulates real-world usage

### Object Naming Convention
**Format:** `{random-prefix}/{base-prefix}-{YYYY-MM-DDTHH-MM-SS-mmm}-{random}[-m]`

**Examples:**
- Regular: `logs/2025/09/test-object-2025-09-30T18-59-33-123-4567` (100B-5KB)
- Multipart: `data/batch-001/daily/test-object-2025-09-30T18-59-33-456-7890-m` (70MB)

### Statistics Tracking
- Real-time stats every 10 seconds
- Final comprehensive report
- Tracks all operation types including errors

## Configuration Options

| Flag           | Default          | Description                           |
|----------------|------------------|---------------------------------------|
| `--endpoint`   | `localhost:9000` | MinIO server endpoint                 |
| `--access-key` | `""`             | MinIO access key                      |
| `--secret-key` | `""`             | MinIO secret key                      |
| `--buckets`    | `test-bucket`    | Target bucket names (comma-separated) |
| `--ssl`        | `false`          | Use HTTPS connection                  |
| `--alias`      | `""`             | Use MC alias from `~/.mc/config.json` |
| `--duration`   | `0`              | Run duration (`0` = infinite)         |
| `--delay`      | `1s`             | Delay between operations              |
| `--prefix`     | `test-object`    | Base object name prefix               |

## Usage Examples

### Basic Usage
```bash
./generate-s3-data \
  --endpoint localhost:9000 \
  --access-key minioadmin \
  --secret-key minioadmin \
  --buckets test-bucket \
  --duration 5m
```

### Using MC Alias
```bash
mc alias set myalias http://localhost:9000 minioadmin minioadmin
./generate-s3-data --alias myalias --duration 10m
```

### High-Frequency Testing
```bash
./generate-s3-data \
  --alias production \
  --buckets stress-test \
  --delay 100ms \
  --duration 1h
```

### Multiple Buckets Support
```bash
# Basic multiple buckets
./generate-s3-data \
  --endpoint localhost:9000 \
  --access-key minioadmin \
  --secret-key minioadmin \
  --buckets "bucket1,bucket2,bucket3" \
  --duration 30m

# With MC alias
./generate-s3-data \
  --alias prod \
  --buckets "logs-prod,logs-staging,logs-dev" \
  --duration 12h
```

**Multiple Bucket Behavior:**
- **Write operations**: Randomly select target bucket
- **Read/Delete operations**: Search across all buckets
- **Bucket creation**: All buckets created automatically
- **Load balancing**: Random distribution across buckets

## Build and Run

```bash
# Build
make build

# Run with default settings
make run

# Run indefinitely
make run-infinite

# Run with SSL
make run-ssl

# Docker
docker build -t generate-s3-data .
docker run --rm generate-s3-data --help
```

## Operation Details

### Write Operations
- Creates objects with random sizes (100B-5KB)
- Uses random prefixes for distribution
- Generates realistic content patterns

### Read Operations  
- Randomly selects existing objects
- Falls back to write operation if no objects exist
- Reports bytes read

### Overwrite Operations
- Updates existing objects with new content
- Maintains same object key, updates content and metadata
- Creates new object if none exist

### Delete Operations
- Removes individual objects randomly
- Creates object first if none exist for deletion
- Single object removal

### Prefix Delete Operations
- **Groups objects by prefix** (first 2 directory levels)
- **Selects prefix with most objects** for maximum impact
- **Bulk deletes all objects** under selected prefix
- **Reports prefix and object count deleted**

### Multipart Write Operations
- Creates **70MB objects** to guarantee multipart upload
- Uses **5MB part size** to force multipart protocol
- Objects marked with **`-m` suffix** for identification
- Tests S3 multipart upload performance

## Statistics Output

### Real-time (every 10 seconds)
```
[STATS] Read=15, Write=12, Overwrite=8, Delete=10, PrefixDel=3, Multipart=2, Errors=2
```

### Final Report
```
Read Operations:         15
Write Operations:        12  
Overwrite Operations:    8
Delete Operations:       10
Prefix Delete Operations:3
Multipart Operations:    2
Error Operations:        2
Total Operations:        52
```

## Common Use Cases

### Performance Testing
- **Load Testing**: High-frequency operations with minimal delay
- **Multipart Testing**: Large object upload performance
- **Distributed Load**: Random prefixes prevent hotspots

### Audit and Compliance
- **Audit Log Generation**: Creates comprehensive S3 operation logs
- **Compliance Testing**: Validates data retention and deletion policies
- **Activity Simulation**: Generates realistic S3 usage patterns

### Development and QA
- **Integration Testing**: Validates S3 client applications
- **Error Simulation**: Tests error handling and recovery
- **Data Lifecycle Testing**: Complete CRUD operations

## Technical Implementation

### Architecture
- **Go-based**: Single binary, cross-platform
- **MinIO Go SDK**: Official MinIO client library
- **Concurrent Safe**: Thread-safe statistics tracking
- **Context-aware**: Proper cancellation and timeouts

### Error Handling
- **Graceful Failures**: Continues operation on individual failures
- **Error Reporting**: Comprehensive error statistics
- **Fallback Logic**: Creates objects when none exist for operations

### Content Generation
- **Efficient Patterns**: Uses pattern-based generation for large content
- **Size Variation**: Multiple size categories for realistic testing
- **Random Distribution**: Balanced content types and sizes

## Troubleshooting

### Common Issues
1. **Bucket Access**: Ensure credentials have bucket permissions
2. **SSL Errors**: Verify SSL configuration matches endpoint
3. **MC Alias**: Run `mc alias set` before using `--alias` flag
4. **Large Objects**: Multipart operations require more time

### Debug Tips
- Check bucket permissions with `mc ls`
- Verify connectivity with `mc admin info`
- Monitor operations with real-time stats
- Use shorter duration for initial testing

## Integration with MinIO

### Audit Logging
- Generates comprehensive audit trails
- Tests audit log system performance
- Validates audit log completeness

### Performance Metrics
- Tests read/write throughput
- Validates multipart upload performance
- Measures prefix-based load distribution

### Storage Analytics
- Creates varied object sizes for analysis
- Tests storage efficiency with different patterns
- Validates lifecycle policy effectiveness

This tool is essential for comprehensive MinIO testing, providing realistic S3 workloads with detailed analytics and flexible configuration options.