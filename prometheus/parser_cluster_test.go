package main

import (
	"io/ioutil"
	"os"
	"testing"
)

func TestParseClusterMetrics(t *testing.T) {
	content := `# HELP minio_cluster_usage_object_total Total number of objects in a cluster
# TYPE minio_cluster_usage_object_total gauge
minio_cluster_usage_object_total{server="s1"} 12345
# HELP minio_cluster_usage_total_bytes Total cluster usage in bytes
# TYPE minio_cluster_usage_total_bytes gauge
minio_cluster_usage_total_bytes{server="s1"} 5.67e+08
# HELP minio_cluster_objects_size_distribution Distribution of object sizes across a cluster
# TYPE minio_cluster_objects_size_distribution gauge
minio_cluster_objects_size_distribution{range="BETWEEN_1024B_AND_1_MB",server="s1"} 100
minio_cluster_objects_size_distribution{range="BETWEEN_1024_B_AND_64_KB",server="s1"} 200
minio_cluster_objects_version_distribution{range="SINGLE_VERSION",server="s1"} 300
`
	tmpfile, err := ioutil.TempFile("", "cluster_metrics_*.txt")
	if err != nil {
		t.Fatalf("unable to create tmp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(content); err != nil {
		t.Fatalf("unable to write tmp file: %v", err)
	}
	tmpfile.Close()

	mp := NewMetricParser()
	if err := mp.ParseFile(tmpfile.Name()); err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}

	if mp.ClusterObjects != 12345 {
		t.Fatalf("expected ClusterObjects 12345, got %d", mp.ClusterObjects)
	}
	if mp.ClusterBytes != 567000000 {
		t.Fatalf("expected ClusterBytes approx 567000000, got %d", mp.ClusterBytes)
	}

	// normalized keys should exist
	// dump keys for debugging
	for k := range mp.ClusterSizeDist {
		t.Logf("ClusterSizeDist key: %s", k)
	}
	for k := range mp.ClusterVersionDist {
		t.Logf("ClusterVersionDist key: %s", k)
	}
	// log normalization result for the raw input
	t.Logf("normalizeRange(BETWEEN_1024B_AND_1_MB) => %s", normalizeRange("BETWEEN_1024B_AND_1_MB"))
	if _, ok := mp.ClusterSizeDist["BETWEEN_1024_B_AND_1_MB"]; !ok {
		t.Fatalf("expected normalized key BETWEEN_1024_B_AND_1_MB in ClusterSizeDist")
	}
	if _, ok := mp.ClusterSizeDist["BETWEEN_1024_B_AND_64_KB"]; !ok {
		t.Fatalf("expected key BETWEEN_1024_B_AND_64_KB in ClusterSizeDist")
	}
	if _, ok := mp.ClusterVersionDist["SINGLE_VERSION"]; !ok {
		t.Fatalf("expected SINGLE_VERSION in ClusterVersionDist")
	}
}
