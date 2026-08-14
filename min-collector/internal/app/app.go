// Package app wires the configured collectors together and owns their
// lifecycle.
package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jkandasa/min-tools/min-collector/internal/config"
	"github.com/jkandasa/min-tools/min-collector/internal/logx"
	"github.com/jkandasa/min-tools/min-collector/internal/metrics"
	"github.com/jkandasa/min-tools/min-collector/internal/rotate"
	"github.com/jkandasa/min-tools/min-collector/internal/webhook"
)

// shutdownTimeout bounds the graceful shutdown of the webhook server.
const shutdownTimeout = 10 * time.Second

// App holds the collectors selected by the configuration.
type App struct {
	config    *config.Config
	server    *webhook.Server
	collector *metrics.Collector
}

// New builds the app, creating the output directories and files up front so a
// permission problem is reported at startup rather than on the first event.
func New(cfg *config.Config) (*App, error) {
	app := &App{config: cfg}

	if err := os.MkdirAll(cfg.Storage.OutputDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create output directory %s: %w", cfg.Storage.OutputDir, err)
	}

	if cfg.AnyWebhookEnabled() {
		app.server = webhook.NewServer(cfg.ListenAddress, config.DefaultHealthPath)

		for _, stream := range []config.Webhook{cfg.Logger, cfg.Audit} {
			if !stream.Enabled {
				continue
			}

			writer, err := rotate.New(rotate.Options{
				Dir:           cfg.StreamDir(stream.Name),
				Name:          stream.Name,
				Ext:           "ndjson",
				MaxSize:       cfg.Storage.MaxFileSize,
				RetainCount:   cfg.Storage.RetainCount,
				Compression:   cfg.Storage.Compression,
				FlushInterval: cfg.Storage.FlushInterval,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to prepare %s stream: %w", stream.Name, err)
			}

			app.server.Handle(webhook.NewReceiver(stream.Name, stream.Path, stream.AuthToken, writer))
		}
	}

	if cfg.Metrics.Enabled {
		app.collector = metrics.New(cfg, cfg.StreamDir("metrics"))
	}

	return app, nil
}

// Run starts every enabled collector and blocks until ctx is cancelled or a
// collector fails.
func (a *App) Run(ctx context.Context) error {
	a.banner()

	errCh := make(chan error, 2)

	if a.server != nil {
		go func() {
			errCh <- a.server.ListenAndServe()
		}()
	}

	if a.collector != nil {
		// Only the metrics scrape is running, so a broken endpoint should stop
		// the process rather than leave it collecting nothing.
		failFast := a.server == nil
		go func() {
			errCh <- a.collector.Run(ctx, failFast)
		}()
	}

	if a.config.StatsInterval > 0 {
		go a.reportStats(ctx)
	}

	var runErr error
	select {
	case <-ctx.Done():
		logx.Infof("shutting down")
	case runErr = <-errCh:
		if runErr != nil {
			logx.Errorf("%v", runErr)
		}
	}

	a.shutdown()
	return runErr
}

// shutdown stops the server and flushes every file.
func (a *App) shutdown() {
	if a.server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := a.server.Shutdown(shutdownCtx); err != nil {
			logx.Warnf("webhook server did not stop cleanly: %v", err)
		}
		for _, receiver := range a.server.Receivers() {
			if err := receiver.Close(); err != nil {
				logx.Warnf("failed to close %s stream: %v", receiver.Name(), err)
			}
		}
	}

	if a.collector != nil {
		if err := a.collector.Close(); err != nil {
			logx.Warnf("failed to close metrics files: %v", err)
		}
	}

	a.logStats()
	logx.Infof("stopped, data is in %s, read rotated files with %s",
		a.config.Storage.OutputDir, a.config.Storage.Compression.Tool())
}

func (a *App) reportStats(ctx context.Context) {
	ticker := time.NewTicker(a.config.StatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.logStats()
		case <-ctx.Done():
			return
		}
	}
}

func (a *App) logStats() {
	var parts []string

	if a.server != nil {
		for _, receiver := range a.server.Receivers() {
			events, rejected, errs := receiver.Stats()
			part := fmt.Sprintf("%s=%d events", receiver.Name(), events)
			if rejected > 0 {
				part += fmt.Sprintf(" (%d rejected)", rejected)
			}
			if errs > 0 {
				part += fmt.Sprintf(" (%d errors)", errs)
			}
			parts = append(parts, part)
		}
	}

	if a.collector != nil {
		scrapes, samples, failures := a.collector.Stats()
		part := fmt.Sprintf("metrics=%d scrapes, %d samples", scrapes, samples)
		if failures > 0 {
			part += fmt.Sprintf(" (%d failed)", failures)
		}
		parts = append(parts, part)
	}

	if len(parts) > 0 {
		logx.Infof("collected: %s", strings.Join(parts, " | "))
	}
}

// banner prints what is enabled and the exact commands needed to point MinIO
// at this collector.
func (a *App) banner() {
	cfg := a.config

	logx.Infof("min-collector %s starting", config.Version)
	logx.Infof("output directory: %s", cfg.Storage.OutputDir)
	logx.Infof("rotation: %d MiB per file, retain %s, compression %s",
		cfg.Storage.MaxFileSize/(1024*1024), retainText(cfg.Storage.RetainCount), cfg.Storage.Compression)

	if a.server != nil {
		for _, receiver := range a.server.Receivers() {
			logx.Infof("%s webhook listening on %s%s -> %s", receiver.Name(), cfg.ListenAddress, receiver.Path(), receiver.File())
		}
		logx.Infof("health endpoint on %s%s", cfg.ListenAddress, config.DefaultHealthPath)
		if cfg.Logger.AuthToken == "" {
			logx.Warnf("webhook authorization is disabled, set --auth-token to require one")
		}
	}

	if a.collector != nil {
		scope := "all metrics"
		if !cfg.Metrics.All() {
			scope = fmt.Sprintf("%d metric(s)", len(cfg.Metrics.List))
		}
		logx.Infof("metrics scrape every %s from %s, collecting %s", cfg.Metrics.Interval, a.collector.URL(), scope)
		if cfg.Metrics.SchemeAssumed {
			logx.Warnf("--metrics-endpoint has no scheme, assuming https, pass http:// explicitly for a plain HTTP server")
		}
	} else {
		logx.Infof("metrics scrape is disabled, enable it with --metrics")
	}

	a.printWiring()
}

func (a *App) printWiring() {
	if a.server == nil {
		return
	}

	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "collector-host"
	}
	endpoint := "http://" + host + hostPort(a.config.ListenAddress)

	auth := ""
	if a.config.Logger.AuthToken != "" {
		auth = ` auth_token="<auth-token>"`
	}

	logx.Print("")
	logx.Print("Point MinIO at this collector:")
	for _, receiver := range a.server.Receivers() {
		logx.Print(fmt.Sprintf(`  mc admin config set <alias> %s_webhook:min-collector endpoint="%s%s"%s`,
			receiver.Name(), endpoint, receiver.Path(), auth))
	}
	logx.Print("  mc admin service restart <alias>")
	logx.Print("")
}

// hostPort turns a listen address into the port suffix used in a URL.
func hostPort(address string) string {
	if idx := strings.LastIndex(address, ":"); idx >= 0 {
		return address[idx:]
	}
	return ":" + address
}

func retainText(count int) string {
	if count <= 0 {
		return "all files"
	}
	return fmt.Sprintf("%d files", count)
}
