package config

import (
	"flag"
	"fmt"
)

// flagGroups controls how the flags are presented in the help output.
var flagGroups = []struct {
	title string
	flags []string
}{
	{"Configuration file", []string{"config", "print-config"}},
	{"Server", []string{"listen", "auth-token"}},
	{"Logger webhook (enabled by default)", []string{"logger", "logger-path"}},
	{"Audit webhook (enabled by default)", []string{"audit", "audit-path"}},
	{"Metrics scrape (disabled by default)", []string{"metrics", "metrics-endpoint", "metrics-token", "metrics-interval", "metrics-list", "metrics-path", "insecure"}},
	{"Storage, rotation and compression (applies to all streams)", []string{"output-dir", "max-file-size", "retain-count", "compress", "flush-interval"}},
	{"Misc", []string{"stats-interval", "log-level", "log-format", "debug", "version"}},
}

// secretFlags are never printed with their value.
var secretFlags = map[string]bool{"auth-token": true, "metrics-token": true}

func printUsage(fs *flag.FlagSet) {
	out := fs.Output()

	fmt.Fprint(out, `min-collector collects logs, audit events and metrics from a MinIO cluster
and stores them in rotated, compressed files.

Usage:
  min-collector [flags]

Logger and audit are received over HTTP webhooks and are enabled by default.
Metrics are scraped from MinIO and are disabled until --metrics is passed.

A setting is resolved in this order, each step overwriting the previous one:
  defaults -> --config file -> flags -> environment (tokens only)

`)

	for _, group := range flagGroups {
		fmt.Fprintf(out, "%s:\n", group.title)
		for _, name := range group.flags {
			f := fs.Lookup(name)
			if f == nil {
				continue
			}

			value := f.DefValue
			if secretFlags[name] && value != "" {
				value = redactedValue
			}
			if value == "" {
				value = `""`
			}
			fmt.Fprintf(out, "  --%-18s %s (default %s)\n", name, f.Usage, value)
		}
		fmt.Fprintln(out)
	}

	fmt.Fprintf(out, `Environment variables (secrets only, they win over the file and the flags):
  %-32s webhook authorization token
  %-32s JWT for the metrics scrape

Examples:
  # Receive logger and audit webhooks on the default paths
  min-collector --output-dir /var/log/min-collector

  # Same, plus a metrics scrape every 30s
  min-collector --metrics --metrics-endpoint https://minio.example.com \
    --metrics-token "$(mc admin prometheus generate myminio --json | jq -r .token)" \
    --metrics-interval 30s

  # Audit only, on a custom path and port, keeping the last 20 rotated files
  min-collector --logger=false --listen :9500 --audit-path /minio/audit --retain-count 20

  # Everything from a file, with the tokens from the environment
  min-collector --config /etc/min-collector.yaml

  # Write a starting point for that file
  min-collector --print-config > /etc/min-collector.yaml

Point MinIO at this collector:
  mc admin config set <alias> logger_webhook:min-collector endpoint="http://<collector-host>%s%s"
  mc admin config set <alias> audit_webhook:min-collector  endpoint="http://<collector-host>%s%s"
  mc admin service restart <alias>
`,
		EnvAuthToken, EnvMetricsToken,
		DefaultListenAddress, DefaultLoggerPath, DefaultListenAddress, DefaultAuditPath)
}
