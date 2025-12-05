package main

import (
	"context"
	"crypto/tls"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

type MetricsCollector struct {
	config      *Config
	httpClient  *http.Client
	fileWriters map[string]*RotatingFileWriter
}

type PrometheusMetric struct {
	Name    string           `json:"name"`
	Help    string           `json:"help"`
	Type    string           `json:"type"`
	Metrics []MetricInstance `json:"metrics"`
}

type MetricInstance struct {
	Labels map[string]string `json:"labels"`
	Value  interface{}       `json:"value"`
}

type MetricData struct {
	Timestamp  string
	MetricName string
	Labels     map[string]string
	Value      string
}

func NewMetricsCollector(config *Config) *MetricsCollector {
	return &MetricsCollector{
		config: config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		fileWriters: make(map[string]*RotatingFileWriter),
	}
}

func (mc *MetricsCollector) Collect(ctx context.Context) error {
	DebugLog("Collecting metrics at %s", time.Now().Format(time.RFC3339))

	// Fetch metrics from MinIO
	metrics, err := mc.fetchMetrics(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch metrics: %w", err)
	}

	// Process each configured metric
	for _, metricName := range mc.config.Metrics {
		if err := mc.processMetric(metricName, metrics); err != nil {
			log.Printf("Error processing metric %s: %v", metricName, err)
		}
	}

	return nil
}

func (mc *MetricsCollector) fetchMetrics(ctx context.Context) (string, error) {
	// Construct metrics endpoint URL
	endpoint := strings.TrimSuffix(mc.config.MinioEndpoint, "/")
	metricsURL := fmt.Sprintf("%s/minio/v2/metrics/cluster", endpoint)

	req, err := http.NewRequestWithContext(ctx, "GET", metricsURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Add JWT token to Authorization header
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", mc.config.JWTToken))
	req.Header.Set("Accept", "text/plain")

	resp, err := mc.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch metrics: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			log.Printf("Warning: failed to close response body: %v", closeErr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return "", fmt.Errorf("unexpected status code %d, failed to read body: %w", resp.StatusCode, readErr)
		}
		return "", fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return string(body), nil
}

func (mc *MetricsCollector) processMetric(metricName string, prometheusData string) error {
	// Parse Prometheus format metrics
	metricDataList := mc.parsePrometheusMetric(metricName, prometheusData)

	if len(metricDataList) == 0 {
		DebugLog("No data found for metric: %s", metricName)
		return nil
	}

	// Get or create file writer for this metric
	writer, err := mc.getFileWriter(metricName)
	if err != nil {
		return fmt.Errorf("failed to get file writer: %w", err)
	}

	// Write each metric data point
	timestamp := time.Now().Format(time.RFC3339)
	for _, data := range metricDataList {
		data.Timestamp = timestamp

		// Format labels as comma-separated key=value pairs
		var labelPairs []string
		for k, v := range data.Labels {
			labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", k, v))
		}
		// Sort for consistent output
		sort.Strings(labelPairs)
		labelsStr := strings.Join(labelPairs, ",")

		// Use CSV encoder to properly escape values
		var buf strings.Builder
		csvWriter := csv.NewWriter(&buf)
		record := []string{
			data.Timestamp,
			data.MetricName,
			labelsStr,
			data.Value,
		}

		if err := csvWriter.Write(record); err != nil {
			log.Printf("Failed to encode CSV record: %v", err)
			continue
		}
		csvWriter.Flush()

		if err := writer.Write([]byte(buf.String())); err != nil {
			log.Printf("Failed to write metric data: %v", err)
		}
	}

	return nil
}

func (mc *MetricsCollector) parsePrometheusMetric(metricName string, data string) []MetricData {
	var results []MetricData

	lines := strings.Split(data, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check if line starts with the metric name
		if !strings.HasPrefix(line, metricName) {
			continue
		}

		// Parse Prometheus format: metric_name{label="value",label2="value2"} value
		metricData := mc.parsePrometheusLine(line, metricName)
		if metricData != nil {
			results = append(results, *metricData)
		}
	}

	return results
}

func (mc *MetricsCollector) parsePrometheusLine(line string, metricName string) *MetricData {
	// Find the position of '{' and '}'
	braceStart := strings.Index(line, "{")
	braceEnd := strings.Index(line, "}")

	var labels map[string]string

	if braceStart == -1 || braceEnd == -1 {
		// No labels, just metric name and value
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			return &MetricData{
				MetricName: metricName,
				Labels:     make(map[string]string),
				Value:      parts[1],
			}
		}
		return nil
	}

	// Extract labels
	labelsStr := line[braceStart+1 : braceEnd]
	labels = mc.parseLabels(labelsStr)

	// Extract value
	valueStr := strings.TrimSpace(line[braceEnd+1:])
	parts := strings.Fields(valueStr)
	if len(parts) == 0 {
		return nil
	}

	return &MetricData{
		MetricName: metricName,
		Labels:     labels,
		Value:      parts[0],
	}
}

func (mc *MetricsCollector) parseLabels(labelsStr string) map[string]string {
	labels := make(map[string]string)

	// Split by comma, handling quoted values
	var currentKey, currentValue string
	inQuotes := false
	var buffer strings.Builder

	for i := 0; i < len(labelsStr); i++ {
		char := labelsStr[i]

		switch char {
		case '"':
			inQuotes = !inQuotes
		case '=':
			if !inQuotes && currentKey == "" {
				currentKey = strings.TrimSpace(buffer.String())
				buffer.Reset()
			} else {
				buffer.WriteByte(char)
			}
		case ',':
			if !inQuotes {
				currentValue = strings.TrimSpace(buffer.String())
				if currentKey != "" {
					labels[currentKey] = currentValue
				}
				currentKey = ""
				currentValue = ""
				buffer.Reset()
			} else {
				buffer.WriteByte(char)
			}
		default:
			buffer.WriteByte(char)
		}
	}

	// Handle last label
	if currentKey != "" {
		currentValue = strings.TrimSpace(buffer.String())
		labels[currentKey] = currentValue
	}

	return labels
}

func (mc *MetricsCollector) getFileWriter(metricName string) (*RotatingFileWriter, error) {
	if writer, exists := mc.fileWriters[metricName]; exists {
		return writer, nil
	}

	// Create new rotating file writer
	writer := NewRotatingFileWriter(
		mc.config.OutputDir,
		metricName,
		mc.config.MaxFileSize,
		mc.config.RetainFileCount,
	)

	// Write CSV header
	header := "timestamp,metric,labels,value\n"
	if err := writer.Write([]byte(header)); err != nil {
		return nil, fmt.Errorf("failed to write header: %w", err)
	}

	mc.fileWriters[metricName] = writer
	return writer, nil
}

func (mc *MetricsCollector) Close() {
	for _, writer := range mc.fileWriters {
		writer.Close()
	}
}
