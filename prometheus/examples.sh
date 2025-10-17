#!/bin/bash

# Example usage script for MinIO Bucket Summary Tool

echo "=== MinIO Bucket Summary Tool Examples ==="
echo

echo "1. Basic usage with sample data:"
echo "   ./bucket_summary sample.txt"
echo

echo "2. Show top 10 buckets:"
echo "   ./bucket_summary sample.txt 10"
echo

echo "3. Show top 3 buckets:"
echo "   ./bucket_summary sample.txt 3"
echo
echo "4. Include cluster-level aggregates (force cluster summary):"
echo "   ./bucket_summary sample.txt --cluster"
echo

echo "4. Using make commands:"
echo "   make build      # Build the tool"
echo "   make run        # Run with sample data"
echo "   make test       # Run with test data"
echo "   make run-top    # Run showing top 10"
echo "   make clean      # Clean build artifacts"
echo

echo "=== Sample output format ==="
echo "BUCKET NAME                    OBJECT COUNT  SIZE (BYTES)  SIZE (HUMAN)  VERSIONING     SERVERS"
echo "my-bucket-1                    1000          1073741824    1.0 GB        Single Version minio-server1:9000"
echo "my-bucket-2                    2500          5368709120    5.0 GB        Multi-Version  minio-server1:9000"
echo

echo "The tool will:"
echo "• Parse MinIO Prometheus metrics"
echo "• Aggregate data across multiple servers"
echo "• Track object versioning distribution"
echo "• Sort buckets by size (largest first)"
echo "• Show human-readable sizes"
echo "• Display versioning status (Unversioned/Single Version/Multi-Version/Mixed)"
echo "• Display total statistics"
echo "• Highlight top N buckets with detailed version breakdown"
