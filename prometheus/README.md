# MinIO Bucket Summary Tool

This Go program parses MinIO Prometheus metrics and generates a summary table showing bucket-level object counts and sizes.

## Features

- Parses Prometheus metrics format from MinIO
- Aggregates data across multiple servers
- Shows object count and size (bytes and human-readable) per bucket
- **NEW: Tracks object versioning distribution per bucket**
- Sorts buckets by size (largest first)
- Displays total statistics
- Shows top N buckets by size with detailed version information

## Metrics Parsed

The tool specifically looks for these MinIO metrics:
- `minio_bucket_usage_object_total` - Total number of objects per bucket
- `minio_bucket_usage_total_bytes` - Total bucket size in bytes
- **`minio_bucket_objects_version_distribution`** - Object versioning distribution per bucket
- `minio_cluster_usage_object_total` - Total objects in cluster (fallback)
- `minio_cluster_usage_total_bytes` - Total bytes in cluster (fallback)
- `minio_cluster_objects_version_distribution` - Cluster-level version distribution (fallback)
- `minio_cluster_objects_size_distribution` - Cluster-level size distribution (fallback)

### Version Distribution Ranges:
- `UNVERSIONED` - Objects without versioning enabled
- `SINGLE_VERSION` - Objects with exactly one version
- `BETWEEN_2_AND_10` - Objects with 2-10 versions
- `BETWEEN_10_AND_100` - Objects with 10-100 versions  
- `BETWEEN_100_AND_1000` - Objects with 100-1000 versions
- `BETWEEN_1000_AND_10000` - Objects with 1000-10000 versions
- `GREATER_THAN_10000` - Objects with more than 10000 versions

## Usage

### Build the program:
```bash
go build -o bucket_summary bucket_summary.go
```

### Run the program:
```bash
# Basic usage - shows version distribution by default
./bucket_summary sample.txt

# Show version distribution explicitly
./bucket_summary sample.txt --versions

# Show size distribution
./bucket_summary sample.txt --sizes

# Show both version and size distribution
./bucket_summary sample.txt --both

# Include cluster-level aggregates even when per-bucket metrics exist
./bucket_summary sample.txt --cluster

# Show top 10 buckets with size distribution
./bucket_summary sample.txt --sizes 10

# Show top 3 buckets with both distributions
./bucket_summary sample.txt --both 3
```

### Expected Output:

```
Parsing MinIO metrics from: sample.txt
============================================================

Bucket Summary Table:
============================================================
BUCKET NAME                                   OBJECT COUNT  SIZE (BYTES)      SIZE (HUMAN)  VERSIONING     SERVERS
----------------------------------------------------------------------------------------------------
container-registry-prod                       237221        1000742900667     931.2 GB      Multi-Version  minio-node1.example.com:9000
customer-service-data                         42252         143712537755      133.8 GB      Single Version minio-node1.example.com:9000
analytics-reports-uat                          97            6347235931        5.9 GB        Single Version minio-node1.example.com:9000
...
----------------------------------------------------------------------------------------------------
TOTAL (45 buckets)                            892156        1175234567890     1.1 TB

Top 5 Buckets by Size:
==================================================
1. documents-archive-prod
   Objects: 72966775
   Size: 38.5 TB (42302750485687 bytes)
   Versioning: Multi-Version
   Version Details: Single: 71408836, 2-10v: 1557158, 10-100v: 781
   Servers: minio-node1.example.com:9000

2. images-prod
   Objects: 472861818
   Size: 35.3 TB (38799398025033 bytes)
   Versioning: Multi-Version
   Version Details: Single: 472790979, 2-10v: 70797, 10-100v: 26, 100-1Kv: 12, 1K-10Kv: 4
   Servers: minio-node1.example.com:9000
```

## Code Structure

### Main Components:

1. **BucketSummary struct**: Holds bucket information
   - Name: Bucket name
   - ObjectCount: Total number of objects
   - SizeBytes: Total size in bytes
   - SizeHuman: Human-readable size
   - **VersionDistribution: Map tracking object version distribution**
   - Servers: List of servers hosting this bucket

2. **MetricParser struct**: Handles parsing and aggregation
   - Parses Prometheus metrics format
   - Extracts bucket names, server names, and values
   - Aggregates data across servers

3. **Helper functions**:
   - `formatBytes()`: Converts bytes to human-readable format
   - `extractBucketName()`: Extracts bucket name from metric labels
   - `extractServerName()`: Extracts server name from metric labels
   - **`extractRange()`: Extracts version range from metric labels**
   - `extractValue()`: Extracts numeric values (including scientific notation)
   - **`formatVersionDistribution()`: Formats version distribution data**
   - **`getVersioningStatus()`: Determines overall versioning status**

### Key Features:

- **Regex-based parsing**: Uses regular expressions to extract bucket names, server names, and version ranges from Prometheus labels
- **Scientific notation support**: Handles large numbers in scientific notation (e.g., 1.4371253755e+11)
- **Server aggregation**: Combines data from multiple servers for the same bucket
- **Version tracking**: Aggregates version distribution data across servers
- **Smart versioning status**: Determines if buckets are Unversioned, Single Version, Multi-Version, or Mixed
- **Sorting**: Sorts buckets by size in descending order
- **Formatted output**: Uses tabwriter for clean, aligned table output
 - **Range normalization**: The tool normalizes inconsistent range label keys (for example, `BETWEEN_1024B_AND_1_MB` and `BETWEEN_1024_B_AND_1_MB` are treated identically)

## Example Input Format

The tool expects Prometheus metrics in this format:

```
# HELP minio_bucket_usage_object_total Total number of objects
# TYPE minio_bucket_usage_object_total gauge
minio_bucket_usage_object_total{bucket="my-bucket",server="minio-node1.example.com:9000"} 1234

# HELP minio_bucket_usage_total_bytes Total bucket size in bytes
# TYPE minio_bucket_usage_total_bytes gauge
minio_bucket_usage_total_bytes{bucket="my-bucket",server="minio-node1.example.com:9000"} 5.67e+08

# HELP minio_bucket_objects_version_distribution Distribution of object versions in the bucket
# TYPE minio_bucket_objects_version_distribution gauge
minio_bucket_objects_version_distribution{bucket="my-bucket",range="SINGLE_VERSION",server="minio-node1.example.com:9000"} 1000
minio_bucket_objects_version_distribution{bucket="my-bucket",range="BETWEEN_2_AND_10",server="minio-node1.example.com:9000"} 234
```

## Requirements

- Go 1.21 or later
- Input file with MinIO Prometheus metrics

## Error Handling

- Validates command line arguments
- Handles file opening errors
- Gracefully handles parsing errors
- Provides meaningful error messages
