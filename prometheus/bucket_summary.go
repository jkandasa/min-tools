package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

// BucketSummary represents the summary information for a bucket
type BucketSummary struct {
	Name                string
	ObjectCount         int64
	SizeBytes           int64
	SizeHuman           string
	Servers             []string
	VersionDistribution map[string]int64 // Tracks object version distribution
	SizeDistribution    map[string]int64 // Tracks object size distribution
}

// MetricParser parses Prometheus metrics
type MetricParser struct {
	buckets map[string]*BucketSummary
}

// DisplayOptions controls what information to show
type DisplayOptions struct {
	ShowVersions bool // Show version distribution
	ShowSizes    bool // Show size distribution
}

// NewMetricParser creates a new metric parser
func NewMetricParser() *MetricParser {
	return &MetricParser{
		buckets: make(map[string]*BucketSummary),
	}
}

// formatBytes converts bytes to human readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB",
		float64(bytes)/float64(div), "KMGTPE"[exp])
}

// formatVersionDistribution creates a summary of version distribution
func formatVersionDistribution(versionDist map[string]int64) string {
	if len(versionDist) == 0 {
		return "N/A"
	}

	var parts []string

	// Order the ranges for better readability
	rangeOrder := []string{
		"UNVERSIONED",
		"SINGLE_VERSION",
		"BETWEEN_2_AND_10",
		"BETWEEN_10_AND_100",
		"BETWEEN_100_AND_1000",
		"BETWEEN_1000_AND_10000",
		"GREATER_THAN_10000",
	}

	for _, rangeKey := range rangeOrder {
		if count, exists := versionDist[rangeKey]; exists && count > 0 {
			switch rangeKey {
			case "UNVERSIONED":
				parts = append(parts, fmt.Sprintf("Unversioned: %d", count))
			case "SINGLE_VERSION":
				parts = append(parts, fmt.Sprintf("Single: %d", count))
			case "BETWEEN_2_AND_10":
				parts = append(parts, fmt.Sprintf("2-10v: %d", count))
			case "BETWEEN_10_AND_100":
				parts = append(parts, fmt.Sprintf("10-100v: %d", count))
			case "BETWEEN_100_AND_1000":
				parts = append(parts, fmt.Sprintf("100-1Kv: %d", count))
			case "BETWEEN_1000_AND_10000":
				parts = append(parts, fmt.Sprintf("1K-10Kv: %d", count))
			case "GREATER_THAN_10000":
				parts = append(parts, fmt.Sprintf(">10Kv: %d", count))
			}
		}
	}

	if len(parts) == 0 {
		return "All zeros"
	}

	return strings.Join(parts, ", ")
}

// getVersioningStatus provides a simple status based on version distribution
func getVersioningStatus(versionDist map[string]int64) string {
	if len(versionDist) == 0 {
		return "Unknown"
	}

	singleVersion := versionDist["SINGLE_VERSION"]
	unversioned := versionDist["UNVERSIONED"]
	totalVersioned := int64(0)

	for key, count := range versionDist {
		if key != "UNVERSIONED" && key != "SINGLE_VERSION" {
			totalVersioned += count
		}
	}

	if unversioned > 0 && singleVersion == 0 && totalVersioned == 0 {
		return "Unversioned"
	} else if singleVersion > 0 && totalVersioned == 0 {
		return "Single Version"
	} else if totalVersioned > 0 {
		return "Multi-Version"
	} else {
		return "Mixed"
	}
}

// formatSizeDistribution creates a summary of size distribution
func formatSizeDistribution(sizeDist map[string]int64) string {
	if len(sizeDist) == 0 {
		return "N/A"
	}

	var parts []string

	// Order the ranges for better readability (smallest to largest)
	rangeOrder := []string{
		"LESS_THAN_1024_B",
		"BETWEEN_1024_B_AND_1_MB",
		"BETWEEN_1_MB_AND_10_MB",
		"BETWEEN_10_MB_AND_64_MB",
		"BETWEEN_64_MB_AND_128_MB",
		"BETWEEN_128_MB_AND_512_MB",
		"GREATER_THAN_512_MB",
	}

	for _, rangeKey := range rangeOrder {
		if count, exists := sizeDist[rangeKey]; exists && count > 0 {
			switch rangeKey {
			case "LESS_THAN_1024_B":
				parts = append(parts, fmt.Sprintf("<1KB: %d", count))
			case "BETWEEN_1024_B_AND_1_MB":
				parts = append(parts, fmt.Sprintf("1KB-1MB: %d", count))
			case "BETWEEN_1_MB_AND_10_MB":
				parts = append(parts, fmt.Sprintf("1-10MB: %d", count))
			case "BETWEEN_10_MB_AND_64_MB":
				parts = append(parts, fmt.Sprintf("10-64MB: %d", count))
			case "BETWEEN_64_MB_AND_128_MB":
				parts = append(parts, fmt.Sprintf("64-128MB: %d", count))
			case "BETWEEN_128_MB_AND_512_MB":
				parts = append(parts, fmt.Sprintf("128-512MB: %d", count))
			case "GREATER_THAN_512_MB":
				parts = append(parts, fmt.Sprintf(">512MB: %d", count))
			}
		}
	}

	if len(parts) == 0 {
		return "All zeros"
	}

	return strings.Join(parts, ", ")
}

// getSizeStatus provides a simple status based on size distribution
func getSizeStatus(sizeDist map[string]int64) string {
	if len(sizeDist) == 0 {
		return "Unknown"
	}

	small := sizeDist["LESS_THAN_1024_B"] + sizeDist["BETWEEN_1024_B_AND_1_MB"]
	medium := sizeDist["BETWEEN_1_MB_AND_10_MB"] + sizeDist["BETWEEN_10_MB_AND_64_MB"]
	large := sizeDist["BETWEEN_64_MB_AND_128_MB"] + sizeDist["BETWEEN_128_MB_AND_512_MB"] + sizeDist["GREATER_THAN_512_MB"]

	total := small + medium + large
	if total == 0 {
		return "Empty"
	}

	smallPct := float64(small) / float64(total) * 100
	mediumPct := float64(medium) / float64(total) * 100
	largePct := float64(large) / float64(total) * 100

	if smallPct >= 80 {
		return "Mostly Small"
	} else if mediumPct >= 60 {
		return "Mostly Medium"
	} else if largePct >= 60 {
		return "Mostly Large"
	} else {
		return "Mixed Sizes"
	}
}

// extractBucketName extracts bucket name from metric labels
func extractBucketName(line string) string {
	re := regexp.MustCompile(`bucket="([^"]+)"`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// extractServerName extracts server name from metric labels
func extractServerName(line string) string {
	re := regexp.MustCompile(`server="([^"]+)"`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// extractRange extracts range value from metric labels
func extractRange(line string) string {
	re := regexp.MustCompile(`range="([^"]+)"`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// extractValue extracts the metric value from the line
func extractValue(line string) int64 {
	parts := strings.Fields(line)
	if len(parts) > 0 {
		// Get the last part which should be the value
		valueStr := parts[len(parts)-1]
		// Handle scientific notation
		if strings.Contains(valueStr, "e+") {
			if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
				return int64(value)
			}
		} else {
			if value, err := strconv.ParseInt(valueStr, 10, 64); err == nil {
				return value
			}
		}
	}
	return 0
}

// addServer adds a server to the bucket's server list if not already present
func (bs *BucketSummary) addServer(server string) {
	for _, s := range bs.Servers {
		if s == server {
			return
		}
	}
	bs.Servers = append(bs.Servers, server)
}

// ParseFile parses the Prometheus metrics file
func (mp *MetricParser) ParseFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("error opening file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		bucketName := extractBucketName(line)
		if bucketName == "" {
			continue
		}

		serverName := extractServerName(line)

		// Initialize bucket if not exists
		if _, exists := mp.buckets[bucketName]; !exists {
			mp.buckets[bucketName] = &BucketSummary{
				Name:                bucketName,
				Servers:             make([]string, 0),
				VersionDistribution: make(map[string]int64),
				SizeDistribution:    make(map[string]int64),
			}
		}

		bucket := mp.buckets[bucketName]
		bucket.addServer(serverName)

		// Parse object count metrics
		if strings.Contains(line, "minio_bucket_usage_object_total") {
			value := extractValue(line)
			bucket.ObjectCount += value
		}

		// Parse size metrics
		if strings.Contains(line, "minio_bucket_usage_total_bytes") {
			value := extractValue(line)
			bucket.SizeBytes += value
			bucket.SizeHuman = formatBytes(bucket.SizeBytes)
		}

		// Parse version distribution metrics
		if strings.Contains(line, "minio_bucket_objects_version_distribution") {
			rangeValue := extractRange(line)
			if rangeValue != "" {
				value := extractValue(line)
				bucket.VersionDistribution[rangeValue] += value
			}
		}

		// Parse size distribution metrics
		if strings.Contains(line, "minio_bucket_objects_size_distribution") {
			rangeValue := extractRange(line)
			if rangeValue != "" {
				value := extractValue(line)
				bucket.SizeDistribution[rangeValue] += value
			}
		}
	}

	return scanner.Err()
}

// GetSummary returns a sorted list of bucket summaries
func (mp *MetricParser) GetSummary() []*BucketSummary {
	summaries := make([]*BucketSummary, 0, len(mp.buckets))

	for _, bucket := range mp.buckets {
		summaries = append(summaries, bucket)
	}

	// Sort by size (descending)
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].SizeBytes > summaries[j].SizeBytes
	})

	return summaries
}

// PrintSummaryTable prints a formatted table of bucket summaries
func (mp *MetricParser) PrintSummaryTable(opts DisplayOptions) {
	summaries := mp.GetSummary()

	if len(summaries) == 0 {
		fmt.Println("No bucket data found")
		return
	}

	// Create tabwriter for aligned output with proper spacing
	w := tabwriter.NewWriter(os.Stdout, 8, 4, 2, ' ', 0)

	// Print header based on display options
	if opts.ShowVersions && opts.ShowSizes {
		fmt.Fprintln(w, "BUCKET NAME\tOBJECT COUNT\tSIZE (BYTES)\tSIZE (HUMAN)\tVERSIONING\tSIZE DIST")
		fmt.Fprintln(w, "--------\t--------\t--------\t--------\t--------\t--------")
	} else if opts.ShowVersions {
		fmt.Fprintln(w, "BUCKET NAME\tOBJECT COUNT\tSIZE (BYTES)\tSIZE (HUMAN)\tVERSIONING")
		fmt.Fprintln(w, "--------\t--------\t--------\t--------\t--------")
	} else if opts.ShowSizes {
		fmt.Fprintln(w, "BUCKET NAME\tOBJECT COUNT\tSIZE (BYTES)\tSIZE (HUMAN)\tSIZE DIST")
		fmt.Fprintln(w, "--------\t--------\t--------\t--------\t--------")
	} else {
		fmt.Fprintln(w, "BUCKET NAME\tOBJECT COUNT\tSIZE (BYTES)\tSIZE (HUMAN)")
		fmt.Fprintln(w, "--------\t--------\t--------\t--------")
	}

	var totalObjects int64
	var totalBytes int64

	// Print bucket data
	for _, bucket := range summaries {
		// Truncate bucket name if too long
		bucketName := bucket.Name
		if len(bucketName) > 40 {
			bucketName = bucketName[:37] + "..."
		}

		if opts.ShowVersions && opts.ShowSizes {
			versioningStatus := getVersioningStatus(bucket.VersionDistribution)
			sizeStatus := getSizeStatus(bucket.SizeDistribution)
			fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\t%s\n",
				bucketName,
				bucket.ObjectCount,
				bucket.SizeBytes,
				bucket.SizeHuman,
				versioningStatus,
				sizeStatus)
		} else if opts.ShowVersions {
			versioningStatus := getVersioningStatus(bucket.VersionDistribution)
			fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\n",
				bucketName,
				bucket.ObjectCount,
				bucket.SizeBytes,
				bucket.SizeHuman,
				versioningStatus)
		} else if opts.ShowSizes {
			sizeStatus := getSizeStatus(bucket.SizeDistribution)
			fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\n",
				bucketName,
				bucket.ObjectCount,
				bucket.SizeBytes,
				bucket.SizeHuman,
				sizeStatus)
		} else {
			fmt.Fprintf(w, "%s\t%d\t%d\t%s\n",
				bucketName,
				bucket.ObjectCount,
				bucket.SizeBytes,
				bucket.SizeHuman)
		}

		totalObjects += bucket.ObjectCount
		totalBytes += bucket.SizeBytes
	}

	// Print totals
	if opts.ShowVersions && opts.ShowSizes {
		fmt.Fprintln(w, "--------\t--------\t--------\t--------\t--------\t--------")
		fmt.Fprintf(w, "TOTAL (%d buckets)\t%d\t%d\t%s\t\t\n",
			len(summaries),
			totalObjects,
			totalBytes,
			formatBytes(totalBytes))
	} else if opts.ShowVersions || opts.ShowSizes {
		fmt.Fprintln(w, "--------\t--------\t--------\t--------\t--------")
		fmt.Fprintf(w, "TOTAL (%d buckets)\t%d\t%d\t%s\t\n",
			len(summaries),
			totalObjects,
			totalBytes,
			formatBytes(totalBytes))
	} else {
		fmt.Fprintln(w, "--------\t--------\t--------\t--------")
		fmt.Fprintf(w, "TOTAL (%d buckets)\t%d\t%d\t%s\n",
			len(summaries),
			totalObjects,
			totalBytes,
			formatBytes(totalBytes))
	}

	w.Flush()
}

// PrintTopBuckets prints the top N buckets by size
func (mp *MetricParser) PrintTopBuckets(n int, opts DisplayOptions) {
	summaries := mp.GetSummary()

	if len(summaries) == 0 {
		fmt.Println("No bucket data found")
		return
	}

	if n > len(summaries) {
		n = len(summaries)
	}

	fmt.Printf("\nTop %d Buckets by Size:\n", n)
	fmt.Println(strings.Repeat("=", 50))

	for i := 0; i < n; i++ {
		bucket := summaries[i]
		fmt.Printf("%d. %s\n", i+1, bucket.Name)
		fmt.Printf("   Objects: %d\n", bucket.ObjectCount)
		fmt.Printf("   Size: %s (%d bytes)\n", bucket.SizeHuman, bucket.SizeBytes)

		if opts.ShowVersions {
			versioningStatus := getVersioningStatus(bucket.VersionDistribution)
			versionDetail := formatVersionDistribution(bucket.VersionDistribution)
			fmt.Printf("   Versioning: %s\n", versioningStatus)
			if versionDetail != "N/A" && versionDetail != "All zeros" {
				fmt.Printf("   Version Details: %s\n", versionDetail)
			}
		}

		if opts.ShowSizes {
			sizeStatus := getSizeStatus(bucket.SizeDistribution)
			sizeDetail := formatSizeDistribution(bucket.SizeDistribution)
			fmt.Printf("   Size Distribution: %s\n", sizeStatus)
			if sizeDetail != "N/A" && sizeDetail != "All zeros" {
				fmt.Printf("   Size Details: %s\n", sizeDetail)
			}
		}

		fmt.Println()
	}
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "--help" || os.Args[1] == "-h" {
		fmt.Printf("Usage: %s <prometheus_metrics_file> [options] [top_n]\n", os.Args[0])
		fmt.Println("Options:")
		fmt.Println("  --versions    Show version distribution information")
		fmt.Println("  --sizes       Show size distribution information")
		fmt.Println("  --both        Show both version and size distribution")
		fmt.Println("  --help, -h    Show this help message")
		fmt.Println("Examples:")
		fmt.Printf("  %s sample.txt\n", os.Args[0])
		fmt.Printf("  %s sample.txt --versions\n", os.Args[0])
		fmt.Printf("  %s sample.txt --sizes 10\n", os.Args[0])
		fmt.Printf("  %s sample.txt --both 5\n", os.Args[0])
		os.Exit(1)
	}

	var filename string
	var topN = 5 // default
	var opts DisplayOptions

	// Parse command line arguments
	args := os.Args[1:]
	filename = args[0]

	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--versions":
			opts.ShowVersions = true
		case "--sizes":
			opts.ShowSizes = true
		case "--both":
			opts.ShowVersions = true
			opts.ShowSizes = true
		default:
			if n, err := strconv.Atoi(arg); err == nil {
				topN = n
			}
		}
	}

	// Default: show basic columns only (no versions/sizes unless explicitly requested)
	// No default options needed - both ShowVersions and ShowSizes default to false

	parser := NewMetricParser()

	fmt.Printf("Parsing MinIO metrics from: %s\n", filename)
	fmt.Println(strings.Repeat("=", 60))

	if err := parser.ParseFile(filename); err != nil {
		log.Fatalf("Error parsing file: %v", err)
	}

	// Print complete summary table
	fmt.Println("\nBucket Summary Table:")
	fmt.Println(strings.Repeat("=", 60))
	parser.PrintSummaryTable(opts)

	// Print top buckets
	parser.PrintTopBuckets(topN, opts)
}
