package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jkandasa/min-tools/min-collector/internal/compress"
	"github.com/jkandasa/min-tools/min-collector/internal/logx"
)

// writeConfig writes a YAML file and returns its path.
func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "min-collector.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}
	return path
}

const sampleConfig = `
listen: ":9600"
auth_token: file-token
logger:
  enabled: false
audit:
  path: /minio/audit
metrics:
  enabled: true
  endpoint: http://minio.example.com
  token: file-jwt
  interval: 45s
  list:
    - minio_cluster_capacity_usable_free_bytes
storage:
  output_dir: /var/log/min-collector
  max_file_size: 250
  retain_count: 7
  compress: gzip
log:
  level: debug
`

func TestConfigFile(t *testing.T) {
	cfg, err := Parse([]string{"-config", writeConfig(t, sampleConfig)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ListenAddress != ":9600" {
		t.Errorf("expected :9600, got %s", cfg.ListenAddress)
	}
	if cfg.Logger.Enabled {
		t.Error("logger must be disabled by the file")
	}
	if !cfg.Audit.Enabled || cfg.Audit.Path != "/minio/audit" {
		t.Errorf("unexpected audit config %+v", cfg.Audit)
	}
	if cfg.Audit.AuthToken != "file-token" {
		t.Errorf("expected the token from the file, got %q", cfg.Audit.AuthToken)
	}
	if !cfg.Metrics.Enabled || cfg.Metrics.Interval != 45*time.Second {
		t.Errorf("unexpected metrics config %+v", cfg.Metrics)
	}
	if len(cfg.Metrics.List) != 1 {
		t.Errorf("expected 1 metric, got %v", cfg.Metrics.List)
	}
	if cfg.Storage.MaxFileSize != 250*1024*1024 || cfg.Storage.RetainCount != 7 {
		t.Errorf("unexpected storage config %+v", cfg.Storage)
	}
	if cfg.Storage.Compression != compress.Gzip {
		t.Errorf("expected gzip from the file, got %s", cfg.Storage.Compression)
	}
	if !cfg.Debug() {
		t.Error("debug level must be set by the file")
	}
	// Values absent from the file keep their default.
	if cfg.Storage.FlushInterval != 5*time.Second {
		t.Errorf("expected the default flush interval, got %s", cfg.Storage.FlushInterval)
	}
}

func TestFlagsOverrideConfigFile(t *testing.T) {
	cfg, err := Parse([]string{
		"-config", writeConfig(t, sampleConfig),
		"-listen", ":9700",
		"-retain-count", "3",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ListenAddress != ":9700" {
		t.Errorf("the flag must win over the file, got %s", cfg.ListenAddress)
	}
	if cfg.Storage.RetainCount != 3 {
		t.Errorf("the flag must win over the file, got %d", cfg.Storage.RetainCount)
	}
	// An untouched flag must not overwrite the file with its default.
	if cfg.Storage.MaxFileSize != 250*1024*1024 {
		t.Errorf("an unset flag overwrote the file, got %d", cfg.Storage.MaxFileSize)
	}
	if cfg.Logger.Enabled {
		t.Error("an unset flag overwrote logger.enabled from the file")
	}
}

func TestEnvOverridesFlagsAndFile(t *testing.T) {
	t.Setenv(EnvAuthToken, "env-token")
	t.Setenv(EnvMetricsToken, "env-jwt")

	cfg, err := Parse([]string{
		"-config", writeConfig(t, sampleConfig),
		"-auth-token", "flag-token",
		"-metrics-token", "flag-jwt",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Logger.AuthToken != "env-token" || cfg.Audit.AuthToken != "env-token" {
		t.Errorf("expected the token from the environment, got %q", cfg.Audit.AuthToken)
	}
	if cfg.Metrics.Token != "env-jwt" {
		t.Errorf("expected the metrics token from the environment, got %q", cfg.Metrics.Token)
	}
}

func TestOnlyTokensComeFromTheEnvironment(t *testing.T) {
	t.Setenv("MIN_COLLECTOR_LISTEN", ":9999")
	t.Setenv("MIN_COLLECTOR_OUTPUT_DIR", "/should/be/ignored")

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ListenAddress != DefaultListenAddress {
		t.Errorf("only tokens may come from the environment, got %s", cfg.ListenAddress)
	}
	if strings.HasPrefix(cfg.Storage.OutputDir, "/should/be/ignored") {
		t.Errorf("only tokens may come from the environment, got %s", cfg.Storage.OutputDir)
	}
}

func TestConfigFileErrors(t *testing.T) {
	if _, err := Parse([]string{"-config", "/nonexistent/min-collector.yaml"}); err == nil {
		t.Error("expected an error for a missing file")
	}

	_, err := Parse([]string{"-config", writeConfig(t, "listen: \":9600\"\nunknown_key: 1\n")})
	if err == nil || !strings.Contains(err.Error(), "unknown_key") {
		t.Errorf("expected an error naming the unknown key, got %v", err)
	}
}

func TestMetricsListAllFromFile(t *testing.T) {
	cfg, err := Parse([]string{"-config", writeConfig(t, "metrics:\n  list: [all]\n")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Metrics.All() {
		t.Error("expected all metrics to be collected")
	}
}

func TestDefaults(t *testing.T) {
	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.Logger.Enabled || !cfg.Audit.Enabled {
		t.Error("logger and audit must be enabled by default")
	}
	if cfg.Metrics.Enabled {
		t.Error("metrics scrape must be disabled by default")
	}
	if cfg.Logger.Path != DefaultLoggerPath || cfg.Audit.Path != DefaultAuditPath {
		t.Errorf("unexpected default paths %s and %s", cfg.Logger.Path, cfg.Audit.Path)
	}
	if cfg.ListenAddress != DefaultListenAddress {
		t.Errorf("unexpected default listen address %s", cfg.ListenAddress)
	}
	if cfg.Storage.Compression != compress.Zstd {
		t.Errorf("zstd must be the default compression, got %s", cfg.Storage.Compression)
	}
	if cfg.LogLevel != logx.LevelInfo {
		t.Errorf("info must be the default log level, got %s", cfg.LogLevel)
	}
	if cfg.Storage.MaxFileSize != DefaultMaxFileSize*1024*1024 {
		t.Errorf("unexpected default max file size %d", cfg.Storage.MaxFileSize)
	}
}

func TestNormalization(t *testing.T) {
	cfg, err := Parse([]string{
		"-listen", "9500",
		"-audit-path", "minio/audit/",
		"-metrics", "-metrics-endpoint", "minio.example.com/", "-metrics-token", "jwt",
		"-metrics-interval", "30s",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ListenAddress != ":9500" {
		t.Errorf("expected :9500, got %s", cfg.ListenAddress)
	}
	if cfg.Audit.Path != "/minio/audit" {
		t.Errorf("expected /minio/audit, got %s", cfg.Audit.Path)
	}
	if cfg.Metrics.Endpoint != "https://minio.example.com" {
		t.Errorf("expected the https scheme to be added, got %s", cfg.Metrics.Endpoint)
	}
	if !cfg.Metrics.SchemeAssumed {
		t.Error("expected the assumed scheme to be reported")
	}
	if cfg.Metrics.Interval != 30*time.Second {
		t.Errorf("expected 30s, got %s", cfg.Metrics.Interval)
	}
}

func TestMetricsList(t *testing.T) {
	cfg, err := Parse([]string{"-metrics-list", " a , b ,, c "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.Join(cfg.Metrics.List, "|"); got != "a|b|c" {
		t.Errorf("unexpected list %q", got)
	}
	if cfg.Metrics.All() {
		t.Error("an explicit list must not mean all metrics")
	}

	cfg, err = Parse([]string{"-metrics-list", "all"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Metrics.All() {
		t.Error(`"all" must collect every metric`)
	}
}

func TestDoubleDashFlags(t *testing.T) {
	cfg, err := Parse([]string{"--listen", ":9700", "--audit=false", "--compress", "xz"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.ListenAddress != ":9700" {
		t.Errorf("expected :9700, got %s", cfg.ListenAddress)
	}
	if cfg.Audit.Enabled {
		t.Error("--audit=false must disable the audit stream")
	}
	if cfg.Storage.Compression != compress.Xz {
		t.Errorf("expected xz, got %s", cfg.Storage.Compression)
	}
}

func TestCompressionFormats(t *testing.T) {
	tests := []struct {
		value string
		want  compress.Format
	}{
		{"zstd", compress.Zstd},
		{"gzip", compress.Gzip},
		{"gz", compress.Gzip},
		{"xz", compress.Xz},
		{"none", compress.None},
		{"false", compress.None},
		{"true", compress.Default},
	}

	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			cfg, err := Parse([]string{"-compress", test.value})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Storage.Compression != test.want {
				t.Errorf("expected %s, got %s", test.want, cfg.Storage.Compression)
			}
		})
	}

	if _, err := Parse([]string{"-compress", "bzip2"}); err == nil {
		t.Error("expected an error for an unknown compression")
	}

	// The same values work in the file, including the boolean form.
	cfg, err := Parse([]string{"-config", writeConfig(t, "storage:\n  compress: true\n")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Storage.Compression != compress.Default {
		t.Errorf("expected the default format, got %s", cfg.Storage.Compression)
	}
}

func TestLogLevel(t *testing.T) {
	for _, level := range logx.Levels {
		cfg, err := Parse([]string{"-log-level", level})
		if err != nil {
			t.Fatalf("unexpected error for %s: %v", level, err)
		}
		if cfg.LogLevel != level {
			t.Errorf("expected %s, got %s", level, cfg.LogLevel)
		}
	}

	if _, err := Parse([]string{"-log-level", "verbose"}); err == nil {
		t.Error("expected an error for an unknown level")
	}

	// -debug is a shortcut and wins over the level it is combined with.
	cfg, err := Parse([]string{"-log-level", "warn", "-debug"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Debug() {
		t.Errorf("-debug must force the debug level, got %s", cfg.LogLevel)
	}
}

func TestValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"nothing enabled", []string{"--logger=false", "--audit=false"}, "nothing to collect"},
		{"metrics without endpoint", []string{"-metrics"}, "--metrics-endpoint is required"},
		{"metrics without token", []string{"-metrics", "-metrics-endpoint", "https://x"}, "--metrics-token is required"},
		{"same path", []string{"-audit-path", "/logger"}, "must differ"},
		{"reserved path", []string{"-audit-path", "/health"}, "reserved"},
		{"bad duration", []string{"-metrics-interval", "5x"}, "invalid --metrics-interval"},
		{"negative retain", []string{"-retain-count", "-1"}, "--retain-count"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(test.args)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("expected error containing %q, got %q", test.want, err)
			}
		})
	}
}
