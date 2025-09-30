package main

import (
	"strings"
	"testing"
	"time"
)

func TestConfigDefaults(t *testing.T) {
	cfg := Config{
		Endpoint:       "localhost:9000",
		Buckets:        "test-bucket",
		Duration:       0,
		OperationDelay: 1 * time.Second,
		ObjectPrefix:   "test-object",
	}

	if cfg.Endpoint != "localhost:9000" {
		t.Errorf("Expected endpoint localhost:9000, got %s", cfg.Endpoint)
	}

	if cfg.Buckets != "test-bucket" {
		t.Errorf("Expected buckets test-bucket, got %s", cfg.Buckets)
	}

	if cfg.Duration != 0 {
		t.Errorf("Expected duration 0, got %v", cfg.Duration)
	}

	if cfg.OperationDelay != 1*time.Second {
		t.Errorf("Expected delay 1s, got %v", cfg.OperationDelay)
	}
}

func TestStatsInitialization(t *testing.T) {
	stats := &Stats{}

	if stats.ReadOps != 0 {
		t.Errorf("Expected ReadOps to be 0, got %d", stats.ReadOps)
	}

	if stats.WriteOps != 0 {
		t.Errorf("Expected WriteOps to be 0, got %d", stats.WriteOps)
	}

	if stats.OverwriteOps != 0 {
		t.Errorf("Expected OverwriteOps to be 0, got %d", stats.OverwriteOps)
	}

	if stats.DeleteOps != 0 {
		t.Errorf("Expected DeleteOps to be 0, got %d", stats.DeleteOps)
	}

	if stats.PrefixDeleteOps != 0 {
		t.Errorf("Expected PrefixDeleteOps to be 0, got %d", stats.PrefixDeleteOps)
	}

	if stats.MultipartOps != 0 {
		t.Errorf("Expected MultipartOps to be 0, got %d", stats.MultipartOps)
	}

	if stats.ErrorOps != 0 {
		t.Errorf("Expected ErrorOps to be 0, got %d", stats.ErrorOps)
	}
}

func TestObjectNameGeneration(t *testing.T) {
	client := &MinioClient{
		config: Config{ObjectPrefix: "test"},
	}

	name1 := client.generateObjectName()
	name2 := client.generateObjectName()

	if name1 == name2 {
		t.Error("Generated object names should be unique")
	}

	if len(name1) == 0 {
		t.Error("Generated object name should not be empty")
	}
}

func TestRandomContentGeneration(t *testing.T) {
	client := &MinioClient{}

	content1 := client.generateRandomContent()
	content2 := client.generateRandomContent()

	if len(content1) == 0 {
		t.Error("Generated content should not be empty")
	}

	if len(content2) == 0 {
		t.Error("Generated content should not be empty")
	}

	// Content should be variable (different sizes or content)
	if content1 == content2 && len(content1) == len(content2) {
		t.Log("Warning: Generated content is identical, but this could be random chance")
	}
}

func TestMCConfigParsing(t *testing.T) {
	// Test with a non-existent alias
	_, err := readMCConfig("nonexistent-alias-test")
	if err == nil {
		t.Error("Expected error for non-existent alias")
	}

	// The error message should mention the alias not being found
	if !strings.Contains(err.Error(), "not found in MC config") && !strings.Contains(err.Error(), "config file not found") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestParseBuckets(t *testing.T) {
	client := &MinioClient{}

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single bucket",
			input:    "bucket1",
			expected: []string{"bucket1"},
		},
		{
			name:     "multiple buckets",
			input:    "bucket1,bucket2,bucket3",
			expected: []string{"bucket1", "bucket2", "bucket3"},
		},
		{
			name:     "buckets with spaces",
			input:    "bucket1, bucket2 , bucket3",
			expected: []string{"bucket1", "bucket2", "bucket3"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "buckets with empty values",
			input:    "bucket1,,bucket2,",
			expected: []string{"bucket1", "bucket2"},
		},
		{
			name:     "single bucket with trailing comma",
			input:    "bucket1,",
			expected: []string{"bucket1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client.config.Buckets = tt.input
			result := client.parseBuckets()

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d buckets, got %d", len(tt.expected), len(result))
				return
			}

			for i, expected := range tt.expected {
				if result[i] != expected {
					t.Errorf("Expected bucket[%d] to be %s, got %s", i, expected, result[i])
				}
			}
		})
	}
}

func TestGetRandomBucket(t *testing.T) {
	client := &MinioClient{}

	tests := []struct {
		name        string
		buckets     string
		expectError bool
	}{
		{
			name:        "single bucket",
			buckets:     "bucket1",
			expectError: false,
		},
		{
			name:        "multiple buckets",
			buckets:     "bucket1,bucket2,bucket3",
			expectError: false,
		},
		{
			name:        "no buckets",
			buckets:     "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client.config.Buckets = tt.buckets
			bucket, err := client.getRandomBucket()

			if tt.expectError {
				if err == nil {
					t.Error("Expected error for empty buckets")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			expectedBuckets := client.parseBuckets()
			found := false
			for _, expected := range expectedBuckets {
				if bucket == expected {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("Returned bucket %s not in expected buckets %v", bucket, expectedBuckets)
			}
		})
	}
}

func TestGetRandomBucketDistribution(t *testing.T) {
	client := &MinioClient{
		config: Config{
			Buckets: "bucket1,bucket2,bucket3",
		},
	}

	// Run multiple times to check if all buckets can be selected
	bucketCounts := make(map[string]int)
	iterations := 100

	for i := 0; i < iterations; i++ {
		bucket, err := client.getRandomBucket()
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		bucketCounts[bucket]++
	}

	// Check that all buckets were selected at least once (with high probability)
	expectedBuckets := []string{"bucket1", "bucket2", "bucket3"}
	for _, expected := range expectedBuckets {
		if bucketCounts[expected] == 0 {
			t.Errorf("Bucket %s was never selected in %d iterations", expected, iterations)
		}
	}

	// Check that no unexpected buckets were selected
	for bucket := range bucketCounts {
		found := false
		for _, expected := range expectedBuckets {
			if bucket == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Unexpected bucket %s was selected", bucket)
		}
	}
}

func TestObjectInfo(t *testing.T) {
	obj := ObjectInfo{
		Bucket: "test-bucket",
		Key:    "test/object.txt",
	}

	if obj.Bucket != "test-bucket" {
		t.Errorf("Expected bucket test-bucket, got %s", obj.Bucket)
	}

	if obj.Key != "test/object.txt" {
		t.Errorf("Expected key test/object.txt, got %s", obj.Key)
	}
}
