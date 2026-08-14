// Package config holds the runtime configuration of min-collector and the
// YAML file, command line and environment parsing that produces it.
//
// The value of a setting is resolved in this order, each step overwriting the
// previous one:
//
//	built in defaults -> --config file -> flags -> environment (tokens only)
package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jkandasa/min-tools/min-collector/internal/compress"
	"github.com/jkandasa/min-tools/min-collector/internal/logx"
)

// Defaults used when neither the config file nor a flag supplies a value.
const (
	DefaultListenAddress = ":8080"
	DefaultLoggerPath    = "/logger"
	DefaultAuditPath     = "/audit"
	DefaultHealthPath    = "/health"
	DefaultOutputDir     = "./collected"
	DefaultMetricsPath   = "/minio/v2/metrics/cluster"
	DefaultMetricsList   = "minio_node_drive_total_bytes,minio_node_drive_used_bytes"
	DefaultMaxFileSize   = 100 // MiB
	DefaultInterval      = "1m"
	DefaultFlushInterval = "5s"
	DefaultStatsInterval = "5m"
)

// AllMetrics is the value of --metrics-list that collects every exposed metric.
const AllMetrics = "all"

// redactedValue replaces a token when the configuration is printed.
const redactedValue = "<redacted>"

// Environment variables recognised by min-collector. Only secrets are read
// from the environment, and they win over the config file and the flags.
const (
	EnvAuthToken    = "MIN_COLLECTOR_AUTH_TOKEN"
	EnvMetricsToken = "MIN_COLLECTOR_METRICS_TOKEN"
)

// Webhook is the configuration of a single push based stream received from
// MinIO, that is the logger or the audit webhook.
type Webhook struct {
	// Name identifies the stream, used for the sub directory and file name.
	Name string
	// Enabled turns the receiver on.
	Enabled bool
	// Path is the HTTP path MinIO posts to.
	Path string
	// AuthToken, when set, must match the Authorization header sent by MinIO.
	AuthToken string
}

// Metrics is the configuration of the pull based Prometheus scraper.
type Metrics struct {
	// Enabled turns the scraper on. It is off unless the user asks for it.
	Enabled bool
	// Endpoint is the MinIO server URL, for example https://minio.example.com.
	Endpoint string
	// Token is the JWT produced by "mc admin prometheus generate".
	Token string
	// ScrapePath is the metrics path on the MinIO server.
	ScrapePath string
	// Interval is how often metrics are scraped.
	Interval time.Duration
	// List holds the metric names to keep, empty means every metric.
	List []string
	// Insecure skips TLS certificate verification.
	Insecure bool
	// SchemeAssumed is set when the endpoint was given without a scheme and
	// https was assumed, so the user can be told about it.
	SchemeAssumed bool
}

// All reports whether every exposed metric is collected.
func (m Metrics) All() bool { return len(m.List) == 0 }

// Storage holds the settings shared by every stream written to disk.
type Storage struct {
	// OutputDir is the root directory of the collected data.
	OutputDir string
	// MaxFileSize is the size in bytes at which a file is rotated.
	MaxFileSize int64
	// RetainCount is the number of rotated files kept per stream.
	RetainCount int
	// Compression is applied to rotated files.
	Compression compress.Format
	// FlushInterval is how often buffered records are flushed to disk.
	FlushInterval time.Duration
}

// Config is the complete runtime configuration.
type Config struct {
	ListenAddress string
	Storage       Storage
	Logger        Webhook
	Audit         Webhook
	Metrics       Metrics
	StatsInterval time.Duration
	LogLevel      string
	LogFormat     string
}

// AnyWebhookEnabled reports whether the HTTP server needs to be started.
func (c *Config) AnyWebhookEnabled() bool {
	return c.Logger.Enabled || c.Audit.Enabled
}

// StreamDir returns the directory holding a stream.
func (c *Config) StreamDir(name string) string {
	return filepath.Join(c.Storage.OutputDir, name)
}

// Version information, injected at build time.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Parse resolves the configuration from the config file, the command line and
// the environment. It exits the process for --help, --version and --print-config,
// and returns an error for invalid input.
func Parse(args []string) (*Config, error) {
	base := defaultSettings()

	fs := flag.NewFlagSet("min-collector", flag.ExitOnError)
	fs.Usage = func() { printUsage(fs) }
	values := registerFlags(fs, base)

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if *values.showVersion {
		fmt.Printf("min-collector %s\n", Version)
		fmt.Printf("  commit: %s\n", Commit)
		fmt.Printf("  built:  %s\n", BuildDate)
		os.Exit(0)
	}

	// Defaults first, then the file, then the flags that were passed, then
	// the environment.
	resolved := base
	if path := strings.TrimSpace(*values.configFile); path != "" {
		if err := applyFile(path, &resolved); err != nil {
			return nil, err
		}
	}
	if err := applyFlags(fs, values, &resolved); err != nil {
		return nil, err
	}
	applyEnv(&resolved)

	if *values.printConfig {
		rendered, err := resolved.yamlWithRedactedTokens()
		if err != nil {
			return nil, err
		}
		fmt.Print(rendered)
		os.Exit(0)
	}

	config, err := resolved.toConfig()
	if err != nil {
		return nil, err
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	return config, nil
}

// toConfig converts the resolved settings into the runtime configuration,
// parsing and normalising the values on the way.
func (s settings) toConfig() (*Config, error) {
	flushInterval, err := parseDuration("flush-interval", s.Storage.FlushInterval)
	if err != nil {
		return nil, err
	}
	scrapeInterval, err := parseDuration("metrics-interval", s.Metrics.Interval)
	if err != nil {
		return nil, err
	}
	summaryInterval, err := parseDuration("stats-interval", s.Log.StatsInterval)
	if err != nil {
		return nil, err
	}

	outputDir, err := filepath.Abs(s.Storage.OutputDir)
	if err != nil {
		return nil, fmt.Errorf("invalid output directory: %w", err)
	}

	endpoint, schemeAssumed := normalizeEndpoint(s.Metrics.Endpoint)
	authToken := strings.TrimSpace(s.AuthToken)

	return &Config{
		ListenAddress: normalizeListen(s.Listen),
		Storage: Storage{
			OutputDir:     outputDir,
			MaxFileSize:   s.Storage.MaxFileSize * 1024 * 1024,
			RetainCount:   s.Storage.RetainCount,
			Compression:   s.Storage.Compress,
			FlushInterval: flushInterval,
		},
		Logger: Webhook{
			Name:      "logger",
			Enabled:   s.Logger.Enabled,
			Path:      normalizePath(s.Logger.Path),
			AuthToken: authToken,
		},
		Audit: Webhook{
			Name:      "audit",
			Enabled:   s.Audit.Enabled,
			Path:      normalizePath(s.Audit.Path),
			AuthToken: authToken,
		},
		Metrics: Metrics{
			Enabled:       s.Metrics.Enabled,
			Endpoint:      endpoint,
			Token:         strings.TrimSpace(s.Metrics.Token),
			ScrapePath:    normalizePath(s.Metrics.Path),
			Interval:      scrapeInterval,
			List:          normalizeList(s.Metrics.List),
			Insecure:      s.Metrics.Insecure,
			SchemeAssumed: schemeAssumed,
		},
		StatsInterval: summaryInterval,
		LogLevel:      strings.ToLower(strings.TrimSpace(s.Log.Level)),
		LogFormat:     strings.ToLower(strings.TrimSpace(s.Log.Format)),
	}, nil
}

func (c *Config) validate() error {
	if !c.Logger.Enabled && !c.Audit.Enabled && !c.Metrics.Enabled {
		return fmt.Errorf("nothing to collect: enable at least one of --logger, --audit or --metrics")
	}
	if c.AnyWebhookEnabled() && c.ListenAddress == "" {
		return fmt.Errorf("listen address is required when a webhook is enabled")
	}
	if c.Logger.Enabled && c.Audit.Enabled && c.Logger.Path == c.Audit.Path {
		return fmt.Errorf("logger-path and audit-path must differ, both are %q", c.Logger.Path)
	}
	for _, webhook := range []Webhook{c.Logger, c.Audit} {
		if webhook.Enabled && webhook.Path == DefaultHealthPath {
			return fmt.Errorf("%s-path must not be %q, that path is reserved for the health check", webhook.Name, DefaultHealthPath)
		}
	}
	if c.Metrics.Enabled {
		if c.Metrics.Endpoint == "" {
			return fmt.Errorf("--metrics-endpoint is required with --metrics")
		}
		if c.Metrics.Token == "" {
			return fmt.Errorf("--metrics-token is required with --metrics (or set %s), generate one with \"mc admin prometheus generate <alias>\"", EnvMetricsToken)
		}
		if c.Metrics.Interval <= 0 {
			return fmt.Errorf("--metrics-interval must be positive, for example 30s, 1m or 5m")
		}
	}
	if c.Storage.MaxFileSize <= 0 {
		return fmt.Errorf("--max-file-size must be positive")
	}
	if c.Storage.RetainCount < 0 {
		return fmt.Errorf("--retain-count must not be negative")
	}
	if c.Storage.FlushInterval < 0 {
		return fmt.Errorf("--flush-interval must not be negative")
	}
	if _, err := logx.ParseLevel(c.LogLevel); err != nil {
		return err
	}
	if _, err := logx.ParseFormat(c.LogFormat); err != nil {
		return err
	}
	return nil
}

// Debug reports whether debug logging is on.
func (c *Config) Debug() bool { return c.LogLevel == logx.LevelDebug }

func parseDuration(name, value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return 0, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid --%s %q: use a duration such as 30s, 1m or 1h", name, value)
	}
	if duration < 0 {
		return 0, fmt.Errorf("invalid --%s %q: must not be negative", name, value)
	}
	return duration, nil
}

// parseList splits the comma separated value of --metrics-list.
func parseList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	var list []string
	for item := range strings.SplitSeq(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			list = append(list, item)
		}
	}
	return normalizeList(list)
}

// normalizeList drops empty entries and turns "all" into the empty list, which
// means every metric is collected.
func normalizeList(list []string) []string {
	var cleaned []string
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.EqualFold(item, AllMetrics) {
			return nil
		}
		cleaned = append(cleaned, item)
	}
	return cleaned
}

// normalizePath accepts "audit" as well as "/audit".
func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimSuffix(path, "/")
}

// normalizeListen accepts "8080" as well as ":8080".
func normalizeListen(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	if !strings.Contains(address, ":") {
		if _, err := strconv.Atoi(address); err == nil {
			return ":" + address
		}
	}
	return address
}

// normalizeEndpoint accepts "minio.example.com" as well as a full URL, and
// reports whether the https scheme had to be assumed.
func normalizeEndpoint(endpoint string) (string, bool) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", false
	}

	assumed := false
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
		assumed = true
	}
	return strings.TrimSuffix(endpoint, "/"), assumed
}
