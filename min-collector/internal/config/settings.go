package config

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jkandasa/min-tools/min-collector/internal/compress"
	"github.com/jkandasa/min-tools/min-collector/internal/logx"
)

// settings mirrors the YAML configuration file and holds every value that can
// be configured. It is filled with the defaults, then overwritten by the file,
// then by the flags that were explicitly passed, and finally by the
// environment.
type settings struct {
	Listen    string          `yaml:"listen"`
	AuthToken string          `yaml:"auth_token"`
	Logger    streamSettings  `yaml:"logger"`
	Audit     streamSettings  `yaml:"audit"`
	Metrics   metricsSettings `yaml:"metrics"`
	Storage   storageSettings `yaml:"storage"`
	Log       logSettings     `yaml:"log"`
}

type streamSettings struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

type metricsSettings struct {
	Enabled  bool     `yaml:"enabled"`
	Endpoint string   `yaml:"endpoint"`
	Token    string   `yaml:"token"`
	Path     string   `yaml:"path"`
	Interval string   `yaml:"interval"`
	List     []string `yaml:"list"`
	Insecure bool     `yaml:"insecure"`
}

type storageSettings struct {
	OutputDir     string          `yaml:"output_dir"`
	MaxFileSize   int64           `yaml:"max_file_size"`
	RetainCount   int             `yaml:"retain_count"`
	Compress      compress.Format `yaml:"compress"`
	FlushInterval string          `yaml:"flush_interval"`
}

type logSettings struct {
	Level         string `yaml:"level"`
	Format        string `yaml:"format"`
	StatsInterval string `yaml:"stats_interval"`
}

// defaultSettings returns the configuration used when nothing is supplied.
func defaultSettings() settings {
	return settings{
		Listen: DefaultListenAddress,
		Logger: streamSettings{Enabled: true, Path: DefaultLoggerPath},
		Audit:  streamSettings{Enabled: true, Path: DefaultAuditPath},
		Metrics: metricsSettings{
			Enabled:  false,
			Path:     DefaultMetricsPath,
			Interval: DefaultInterval,
			List:     strings.Split(DefaultMetricsList, ","),
			Insecure: true,
		},
		Storage: storageSettings{
			OutputDir:     DefaultOutputDir,
			MaxFileSize:   DefaultMaxFileSize,
			RetainCount:   0,
			Compress:      compress.Default,
			FlushInterval: DefaultFlushInterval,
		},
		Log: logSettings{
			Level:         logx.LevelInfo,
			Format:        logx.FormatConsole,
			StatsInterval: DefaultStatsInterval,
		},
	}
}

// flagValues holds the value of every flag, used to apply the ones that were
// explicitly passed on top of the file.
type flagValues struct {
	configFile  *string
	printConfig *bool
	showVersion *bool

	listen    *string
	authToken *string

	loggerEnabled *bool
	loggerPath    *string
	auditEnabled  *bool
	auditPath     *string

	metricsEnabled  *bool
	metricsEndpoint *string
	metricsToken    *string
	metricsPath     *string
	metricsInterval *string
	metricsList     *string
	insecure        *bool

	outputDir     *string
	maxFileSize   *int64
	retainCount   *int
	compress      *string
	flushInterval *string

	statsInterval *string
	logLevel      *string
	logFormat     *string
	debug         *bool
}

// registerFlags declares every flag, using base for the documented defaults.
func registerFlags(fs *flag.FlagSet, base settings) *flagValues {
	return &flagValues{
		configFile:  fs.String("config", "", "path to a YAML configuration file"),
		printConfig: fs.Bool("print-config", false, "print the effective configuration as YAML and exit"),
		showVersion: fs.Bool("version", false, "print version information and exit"),

		listen:    fs.String("listen", base.Listen, "address the webhook server listens on, as [host]:port"),
		authToken: fs.String("auth-token", base.AuthToken, "token MinIO must present in the Authorization header, empty disables the check"),

		loggerEnabled: fs.Bool("logger", base.Logger.Enabled, "collect server logs from the MinIO logger webhook"),
		loggerPath:    fs.String("logger-path", base.Logger.Path, "HTTP path that receives the logger webhook"),
		auditEnabled:  fs.Bool("audit", base.Audit.Enabled, "collect audit logs from the MinIO audit webhook"),
		auditPath:     fs.String("audit-path", base.Audit.Path, "HTTP path that receives the audit webhook"),

		metricsEnabled:  fs.Bool("metrics", base.Metrics.Enabled, "scrape Prometheus metrics from MinIO, off by default"),
		metricsEndpoint: fs.String("metrics-endpoint", base.Metrics.Endpoint, "MinIO server URL to scrape, required with --metrics"),
		metricsToken:    fs.String("metrics-token", base.Metrics.Token, "JWT from \"mc admin prometheus generate\", required with --metrics"),
		metricsPath:     fs.String("metrics-path", base.Metrics.Path, "metrics path on the MinIO server"),
		metricsInterval: fs.String("metrics-interval", base.Metrics.Interval, "how often metrics are scraped"),
		metricsList:     fs.String("metrics-list", strings.Join(base.Metrics.List, ","), "metrics to keep, comma separated, or \"all\""),
		insecure:        fs.Bool("insecure", base.Metrics.Insecure, "skip TLS certificate verification when scraping"),

		outputDir:     fs.String("output-dir", base.Storage.OutputDir, "root directory for the collected data"),
		maxFileSize:   fs.Int64("max-file-size", base.Storage.MaxFileSize, "size in MiB at which a file is rotated"),
		retainCount:   fs.Int("retain-count", base.Storage.RetainCount, "rotated files to keep per stream, 0 keeps all"),
		compress:      fs.String("compress", string(base.Storage.Compress), "compression of rotated files: "+compress.Names()),
		flushInterval: fs.String("flush-interval", base.Storage.FlushInterval, "how often buffered records are flushed to disk"),

		statsInterval: fs.String("stats-interval", base.Log.StatsInterval, "how often a collection summary is logged, 0 disables it"),
		logLevel:      fs.String("log-level", base.Log.Level, "log level: "+strings.Join(logx.Levels, ", ")),
		logFormat:     fs.String("log-format", base.Log.Format, "log format: "+strings.Join(logx.Formats, ", ")),
		debug:         fs.Bool("debug", false, "shortcut for --log-level debug"),
	}
}

// applyFile overwrites the settings with the keys present in the YAML file.
// Keys that are absent keep their current value.
func applyFile(path string, target *settings) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			logx.Debugf("failed to close config file: %v", err)
		}
	}()

	decoder := yaml.NewDecoder(file)
	// Reject unknown keys so a typo is reported instead of silently ignored.
	decoder.KnownFields(true)

	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("failed to parse config file %s: %w", path, err)
	}
	return nil
}

// applyFlags overwrites the settings with the flags the user actually passed,
// so a flag wins over the file but an untouched flag does not. It returns an
// error for a flag value that cannot be parsed.
func applyFlags(fs *flag.FlagSet, values *flagValues, target *settings) error {
	var err error

	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "listen":
			target.Listen = *values.listen
		case "auth-token":
			target.AuthToken = *values.authToken

		case "logger":
			target.Logger.Enabled = *values.loggerEnabled
		case "logger-path":
			target.Logger.Path = *values.loggerPath
		case "audit":
			target.Audit.Enabled = *values.auditEnabled
		case "audit-path":
			target.Audit.Path = *values.auditPath

		case "metrics":
			target.Metrics.Enabled = *values.metricsEnabled
		case "metrics-endpoint":
			target.Metrics.Endpoint = *values.metricsEndpoint
		case "metrics-token":
			target.Metrics.Token = *values.metricsToken
		case "metrics-path":
			target.Metrics.Path = *values.metricsPath
		case "metrics-interval":
			target.Metrics.Interval = *values.metricsInterval
		case "metrics-list":
			target.Metrics.List = parseList(*values.metricsList)
		case "insecure":
			target.Metrics.Insecure = *values.insecure

		case "output-dir":
			target.Storage.OutputDir = *values.outputDir
		case "max-file-size":
			target.Storage.MaxFileSize = *values.maxFileSize
		case "retain-count":
			target.Storage.RetainCount = *values.retainCount
		case "compress":
			format, parseErr := compress.Parse(*values.compress)
			if parseErr != nil {
				err = parseErr
				return
			}
			target.Storage.Compress = format
		case "flush-interval":
			target.Storage.FlushInterval = *values.flushInterval

		case "stats-interval":
			target.Log.StatsInterval = *values.statsInterval
		case "log-level":
			target.Log.Level = *values.logLevel
		case "log-format":
			target.Log.Format = *values.logFormat
		}
	})

	// --debug is a shortcut, it wins so that adding it always turns debug on.
	if *values.debug {
		target.Log.Level = logx.LevelDebug
	}

	return err
}

// applyEnv overwrites the tokens from the environment. Only secrets are read
// from the environment, and they win over the file and the flags so they can
// be injected by a service manager without appearing in the command line.
func applyEnv(target *settings) {
	if token, ok := lookupEnv(EnvAuthToken); ok {
		target.AuthToken = token
	}
	if token, ok := lookupEnv(EnvMetricsToken); ok {
		target.Metrics.Token = token
	}
}

func lookupEnv(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(value), true
}

// yamlWithRedactedTokens renders the settings as YAML, keeping secrets out of
// the output so it is safe to print or commit.
func (s settings) yamlWithRedactedTokens() (string, error) {
	redacted := s
	if redacted.AuthToken != "" {
		redacted.AuthToken = redactedValue
	}
	if redacted.Metrics.Token != "" {
		redacted.Metrics.Token = redactedValue
	}

	data, err := yaml.Marshal(redacted)
	if err != nil {
		return "", fmt.Errorf("failed to render the configuration: %w", err)
	}

	header := fmt.Sprintf("# min-collector configuration, use with --config <file>\n"+
		"# tokens are redacted, supply them with %s and %s\n", EnvAuthToken, EnvMetricsToken)
	return header + string(data), nil
}
