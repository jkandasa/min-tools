// Command min-collector collects logs, audit events and metrics from a MinIO
// cluster and stores them in rotated, compressed files.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/jkandasa/min-tools/min-collector/internal/app"
	"github.com/jkandasa/min-tools/min-collector/internal/config"
	"github.com/jkandasa/min-tools/min-collector/internal/logx"
)

func main() {
	cfg, err := config.Parse(os.Args[1:])
	if err != nil {
		logx.Init(logx.LevelInfo, logx.FormatConsole)
		logx.Fatalf("%v, run min-collector --help for usage", err)
	}

	logx.Init(cfg.LogLevel, cfg.LogFormat)
	defer logx.Sync()

	collector, err := app.New(cfg)
	if err != nil {
		logx.Fatalf("%v", err)
	}

	// SIGINT and SIGTERM stop the collectors and flush the open files.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := collector.Run(ctx); err != nil {
		logx.Sync()
		os.Exit(1)
	}
}
