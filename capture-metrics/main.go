package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var (
	// Version information (set via ldflags during build)
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

type Config struct {
	MinioEndpoint   string
	JWTToken        string
	Interval        time.Duration
	MaxFileSize     int64
	RetainFileCount int
	OutputDir       string
	Metrics         []string
	Debug           bool
}

var debugMode bool

// DebugLog prints log messages only when debug mode is enabled
func DebugLog(format string, v ...interface{}) {
	if debugMode {
		log.Printf(format, v...)
	}
}

func main() {
	config := parseFlags()
	debugMode = config.Debug

	if err := validateConfig(config); err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	DebugLog("Starting MinIO Metrics Collector")
	DebugLog("Endpoint: %s", config.MinioEndpoint)
	DebugLog("Interval: %v", config.Interval)
	DebugLog("Max file size: %d MiB", config.MaxFileSize/(1024*1024))
	DebugLog("Retain file count: %d (0 = unlimited)", config.RetainFileCount)
	DebugLog("Metrics: %v", config.Metrics)

	collector := NewMetricsCollector(config)

	// Setup graceful shutdown with context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()

	// Collect metrics immediately on start
	// Fail on first collection error as it could indicate authentication/configuration issues
	if err := collector.Collect(ctx); err != nil {
		log.Fatalf("Initial metrics collection failed: %v", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := collector.Collect(ctx); err != nil {
				log.Printf("Error collecting metrics: %v", err)
			}
		case <-sigChan:
			log.Println("Shutting down gracefully...")
			cancel() // Cancel context to stop any in-progress requests
			collector.Close()
			return
		}
	}
}

func parseFlags() *Config {
	config := &Config{}

	endpoint := flag.String("endpoint", "", "MinIO endpoint URL (required)")
	token := flag.String("token", "", "JWT token for authentication (required)")
	interval := flag.String("interval", "1m", "Collection interval (e.g., 30s, 1m, 5m, 1h)")
	maxFileSize := flag.Int("max-file-size", 100, "Maximum file size in MiB before rotation")
	retainCount := flag.Int("retain-count", 0, "Number of rotated files to keep (0 = keep all)")
	outputDir := flag.String("output-dir", "./metrics", "Directory to store metric files")
	metrics := flag.String("metrics", "minio_node_drive_total_bytes,minio_node_drive_used_bytes", "Comma-separated list of metrics to collect")
	debug := flag.Bool("debug", false, "Enable debug logging")
	showVersion := flag.Bool("version", false, "Show version information")

	flag.Parse()

	// Handle version flag
	if *showVersion {
		fmt.Printf("minio-metrics version %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built:  %s\n", buildDate)
		os.Exit(0)
	}

	config.MinioEndpoint = *endpoint
	config.JWTToken = *token

	// Parse duration string
	duration, err := time.ParseDuration(*interval)
	if err != nil {
		log.Fatalf("Invalid interval format: %v (use format like 30s, 1m, 5m, 1h)", err)
	}
	config.Interval = duration

	config.MaxFileSize = int64(*maxFileSize) * 1024 * 1024 // Convert MiB to bytes
	config.RetainFileCount = *retainCount
	config.OutputDir = *outputDir
	config.Debug = *debug

	// Parse metrics
	if *metrics != "" {
		for _, m := range strings.Split(*metrics, ",") {
			config.Metrics = append(config.Metrics, strings.TrimSpace(m))
		}
	}

	return config
}

func validateConfig(config *Config) error {
	if config.MinioEndpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	if config.JWTToken == "" {
		return fmt.Errorf("token is required")
	}
	if config.Interval <= 0 {
		return fmt.Errorf("interval must be positive (e.g., 30s, 1m, 5m)")
	}
	if config.MaxFileSize <= 0 {
		return fmt.Errorf("max-file-size must be positive")
	}
	if config.RetainFileCount < 0 {
		return fmt.Errorf("retain-count must be non-negative")
	}
	if len(config.Metrics) == 0 {
		return fmt.Errorf("at least one metric must be specified")
	}
	return nil
}
