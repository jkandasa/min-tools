# Generate S3 Data Configuration Examples

This file contains various configuration examples for different use cases.

## Basic Usage with Local MinIO

```bash
./generate-s3-data \
    --endpoint localhost:9000 \
    --access-key minioadmin \
    --secret-key minioadmin \
    --buckets test-bucket \
    --duration 5m
```

## Production Environment with SSL

```bash
./generate-s3-data \
    --endpoint s3.company.com:9000 \
    --access-key YOUR_ACCESS_KEY \
    --secret-key YOUR_SECRET_KEY \
    --buckets audit-logs \
    --ssl \
    --duration 1h \
    --delay 5s
```

## Using MC Alias (MC Config File)

First, configure the alias using MinIO Client:

```bash
mc alias set prod https://s3.company.com:9000 your_access_key your_secret_key
```

Then run:

```bash
./generate-s3-data \
    --alias prod \
    --buckets production-audit \
    --duration 24h \
    --delay 10s
```

## Stress Testing (High Frequency)

```bash
./generate-s3-data \
    --endpoint localhost:9000 \
    --access-key minioadmin \
    --secret-key minioadmin \
    --buckets stress-test \
    --duration 0 \
    --delay 100ms \
    --prefix stress-object
```

## Object Naming Convention

The tool generates objects with **random prefixes** and human-readable timestamps:

**Format:** `{random-prefix}/{base-prefix}-{YYYY-MM-DDTHH-MM-SS-mmm}-{random}[-m]`

**Random Prefix Examples:**
- `data/2025/09/30/`
- `logs/batch-001/monthly/`
- `backup/user-002/q3/prod/`
- `temp/session-a/weekly/`
- `cache/2024/daily/dev/`

**Complete Object Examples:**
- `logs/2025/09/test-object-2025-09-30T18-59-33-123-4567` (regular: 100B-5KB)
- `data/batch-001/daily/test-object-2025-09-30T18-59-33-456-7890-m` (multipart: 70MB)

**Components:**
- **Random prefix**: Simulates directory structure (2-4 levels deep)
- **Base prefix**: Configurable with `--prefix` (default: `test-object`)
- **Timestamp**: `YYYY-MM-DDTHH-MM-SS-mmm` format with milliseconds
- **Random number**: For uniqueness within the same millisecond
- **`-m` suffix**: Indicates multipart upload for large objects

## Long Running Audit (Background)

```bash
nohup ./generate-s3-data \
    --endpoint localhost:9000 \
    --access-key minioadmin \
    --secret-key minioadmin \
    --buckets audit-continuous \
    --duration 0 \
    --delay 30s \
    --prefix audit-$(date +%Y%m%d) \
    > audit.log 2>&1 &
```

## Multiple Buckets Configuration

### Basic Multiple Buckets

```bash
./generate-s3-data \
    --endpoint localhost:9000 \
    --access-key minioadmin \
    --secret-key minioadmin \
    --buckets "bucket1,bucket2,bucket3" \
    --duration 30m \
    --delay 2s
```

### Production Environment with Multiple Buckets

```bash
./generate-s3-data \
    --endpoint s3.company.com:9000 \
    --access-key prod_access_key \
    --secret-key prod_secret_key \
    --buckets "audit-logs,backup-data,temp-storage" \
    --ssl \
    --duration 24h \
    --delay 10s
```

### MC Alias with Multiple Buckets

```bash
# First configure the alias
mc alias set prod https://s3.company.com:9000 access_key secret_key

# Then run with multiple buckets
./generate-s3-data \
    --alias prod \
    --buckets "logs-prod,logs-staging,logs-dev" \
    --duration 12h \
    --delay 5s
```

### Stress Testing Across Multiple Buckets

```bash
./generate-s3-data \
    --endpoint localhost:9000 \
    --access-key minioadmin \
    --secret-key minioadmin \
    --buckets "stress-1,stress-2,stress-3,stress-4,stress-5" \
    --duration 0 \
    --delay 100ms \
    --prefix stress-test
```

**Multiple Bucket Behavior:**
- **Write operations**: Randomly select one of the configured buckets
- **Read/Delete operations**: Search across all configured buckets for existing objects  
- **Bucket creation**: All specified buckets are created automatically if they don't exist
- **Operation logging**: Shows bucket name in logs (e.g., `[SUCCESS] WRITE: bucket2/path/object.txt`)
- **Load distribution**: Operations are distributed randomly across buckets for balanced testing

## Docker with MC Config

When running in Docker, you can mount the MC config directory:

```dockerfile
# Mount MC config directory
VOLUME ["/root/.mc"]

# Or copy existing config
COPY .mc/config.json /root/.mc/config.json
```

Or use direct credentials:
```dockerfile
ENV MINIO_ENDPOINT=minio:9000
ENV MINIO_ACCESS_KEY=minioadmin
ENV MINIO_SECRET_KEY=minioadmin
ENV MINIO_BUCKET=docker-audit
```

## Kubernetes Deployment Example

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: generate-s3-data
spec:
  replicas: 1
  selector:
    matchLabels:
      app: generate-s3-data
  template:
    metadata:
      labels:
        app: generate-s3-data
    spec:
      containers:
      - name: s3-data-generator
        image: your-registry/generate-s3-data:latest
        args:
        - "--endpoint"
        - "minio-service:9000"
        - "--access-key"
        - "$(MINIO_ACCESS_KEY)"
        - "--secret-key"
        - "$(MINIO_SECRET_KEY)"
        - "--bucket"
        - "k8s-audit"
        - "--duration"
        - "0"
        env:
        - name: MINIO_ACCESS_KEY
          valueFrom:
            secretKeyRef:
              name: minio-credentials
              key: access-key
        - name: MINIO_SECRET_KEY
          valueFrom:
            secretKeyRef:
              name: minio-credentials
              key: secret-key
```

## Performance Tuning

### High Throughput
```bash
./generate-s3-data \
    --endpoint localhost:9000 \
    --access-key minioadmin \
    --secret-key minioadmin \
    --bucket perf-test \
    --delay 50ms \
    --prefix perf-object
```

### Low Resource Usage  
```bash
./generate-s3-data \
    --endpoint localhost:9000 \
    --access-key minioadmin \
    --secret-key minioadmin \
    --bucket low-resource \
    --delay 60s \
    --prefix efficient-object
```

## Monitoring and Logging

### With JSON Logging (Future Enhancement)
```bash
./generate-s3-data \
    --endpoint localhost:9000 \
    --access-key minioadmin \
    --secret-key minioadmin \
    --bucket monitor-test \
    --log-format json \
    --log-level info
```

### Redirecting Output
```bash
./generate-s3-data \
    --endpoint localhost:9000 \
    --access-key minioadmin \
    --secret-key minioadmin \
    --bucket log-test 2>&1 | tee audit-$(date +%Y%m%d-%H%M%S).log
```

## Operations Performed

The tool randomly performs the following operations:

1. **WRITE**: Creates small objects (100B-5KB)
2. **READ**: Reads existing objects randomly
3. **OVERWRITE**: Updates existing objects with new content
4. **DELETE**: Removes individual objects randomly
5. **PREFIX DELETE**: Removes all objects under a random prefix (bulk deletion)
6. **MULTIPART WRITE**: Creates large objects (70MB) using guaranteed multipart upload

Objects created with multipart uploads are identified with the `-m` suffix in their names for easy identification during analysis and monitoring.

## Prefix Delete Operation

The **PREFIX DELETE** operation simulates bulk deletion scenarios by removing all objects under a randomly selected prefix path. 

**How it works:**
1. Lists all existing objects in the bucket
2. Groups objects by their prefix (first 2 directory levels)
3. Selects the prefix with the most objects (for better demonstration)
4. Deletes ALL objects under that prefix path

**Example:**
```
Before: 
  data/2025/file1.txt
  data/2025/file2.txt  
  logs/batch-001/log1.txt
  
After PREFIX DELETE of "data/2025/":
  logs/batch-001/log1.txt
  (data/2025/ objects deleted)
```

This operation is valuable for testing:
- **Bulk deletion performance**
- **Directory-level cleanup scenarios** 
- **S3 prefix-based data lifecycle policies**
- **Storage optimization workflows**

## Benefits of Random Prefixes

Random prefixes provide several advantages for S3 testing:

1. **Performance Testing**: Distributes load across multiple S3 partitions
2. **Real-world Simulation**: Mimics actual application usage patterns
3. **Hotspotting Prevention**: Avoids concentrated load on single prefixes
4. **Scaling Validation**: Tests S3's ability to handle distributed workloads
5. **Monitoring Variety**: Creates diverse paths for logging and monitoring tools