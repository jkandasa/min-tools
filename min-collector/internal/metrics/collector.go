// Package metrics scrapes the MinIO Prometheus endpoint and stores the samples
// as CSV.
package metrics

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jkandasa/min-tools/min-collector/internal/config"
	"github.com/jkandasa/min-tools/min-collector/internal/logx"
	"github.com/jkandasa/min-tools/min-collector/internal/rotate"
)

// csvHeader is written at the top of every metrics file.
const csvHeader = "timestamp,metric,labels,value\n"

// allMetricsFile holds every metric when no explicit list is configured.
const allMetricsFile = "all_metrics"

// scrapeTimeout bounds a single scrape.
const scrapeTimeout = 30 * time.Second

// Collector periodically scrapes MinIO and writes the samples to disk.
type Collector struct {
	metrics config.Metrics
	storage config.Storage
	dir     string

	client *http.Client

	mu      sync.Mutex
	writers map[string]*rotate.Writer
	closed  bool

	scrapes  atomic.Int64
	samples  atomic.Int64
	failures atomic.Int64
}

// New creates a Collector writing below dir.
func New(cfg *config.Config, dir string) *Collector {
	return &Collector{
		metrics: cfg.Metrics,
		storage: cfg.Storage,
		dir:     dir,
		client: &http.Client{
			Timeout: scrapeTimeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.Metrics.Insecure}, //nolint:gosec // opt out with --insecure=false
			},
		},
		writers: make(map[string]*rotate.Writer),
	}
}

// URL returns the endpoint that is scraped.
func (c *Collector) URL() string {
	return c.metrics.Endpoint + c.metrics.ScrapePath
}

// Run scrapes immediately and then on every interval until ctx is done.
//
// A failing first scrape usually means the endpoint or the token is wrong, so
// with failFast it is returned as an error and stops the process. When another
// stream is being collected the scrape is only reported and retried, a metrics
// problem must not take the webhooks down with it.
func (c *Collector) Run(ctx context.Context, failFast bool) error {
	if err := c.scrape(ctx); err != nil {
		c.failures.Add(1)
		if failFast {
			return fmt.Errorf("initial metrics scrape failed: %w", err)
		}
		logx.Errorf("initial metrics scrape failed, retrying every %s: %v", c.metrics.Interval, err)
	}

	ticker := time.NewTicker(c.metrics.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := c.scrape(ctx); err != nil {
				c.failures.Add(1)
				logx.Errorf("metrics scrape failed: %v", err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// Stats returns the scrape, sample and failure counters.
func (c *Collector) Stats() (scrapes, samples, failures int64) {
	return c.scrapes.Load(), c.samples.Load(), c.failures.Load()
}

// Close flushes and closes every metrics file.
func (c *Collector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true

	var firstErr error
	for name, writer := range c.writers {
		if err := writer.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to close metrics file %s: %w", name, err)
		}
	}
	c.writers = make(map[string]*rotate.Writer)
	return firstErr
}

func (c *Collector) scrape(ctx context.Context) error {
	started := time.Now()

	payload, err := c.fetch(ctx)
	if err != nil {
		return err
	}

	samples := Parse(payload, c.metrics.List)
	if len(samples) == 0 {
		logx.Warnf("metrics scrape returned no matching samples, check --metrics-list")
		return nil
	}

	timestamp := started.UTC().Format(time.RFC3339)

	// Group by target file first so each file is written once per scrape.
	batches := make(map[string]*bytes.Buffer)
	for _, sample := range samples {
		file := c.fileFor(sample.Name)

		buf, ok := batches[file]
		if !ok {
			buf = &bytes.Buffer{}
			batches[file] = buf
		}

		writer := csv.NewWriter(buf)
		if err := writer.Write([]string{timestamp, sample.Name, sample.LabelString(), sample.Value}); err != nil {
			logx.Warnf("failed to encode sample %s: %v", sample.Name, err)
			continue
		}
		writer.Flush()
	}

	for file, buf := range batches {
		writer, err := c.writerFor(file)
		if err != nil {
			return err
		}
		if err := writer.Write(buf.Bytes()); err != nil {
			logx.Errorf("failed to write metrics: %v", err)
		}
	}

	c.scrapes.Add(1)
	c.samples.Add(int64(len(samples)))
	logx.Debugf("scraped %d samples into %d file(s) in %s", len(samples), len(batches), time.Since(started).Round(time.Millisecond))
	return nil
}

func (c *Collector) fetch(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.metrics.Token)
	req.Header.Set("Accept", "text/plain")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to reach %s: %w", c.URL(), err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logx.Debugf("failed to close response body: %v", err)
		}
	}()

	body, readErr := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		hint := ""
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
			hint = ", regenerate the token with \"mc admin prometheus generate <alias>\""
		}
		return "", fmt.Errorf("%s returned %s%s: %s", c.URL(), resp.Status, hint, strings.TrimSpace(string(body)))
	}
	if readErr != nil {
		return "", fmt.Errorf("failed to read response: %w", readErr)
	}

	return string(body), nil
}

// fileFor maps a metric name to its file. A configured list keeps one file per
// metric, collecting everything keeps a single file to avoid hundreds of them.
func (c *Collector) fileFor(metricName string) string {
	if c.metrics.All() {
		return allMetricsFile
	}
	return metricName
}

func (c *Collector) writerFor(name string) (*rotate.Writer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// A scrape that is still finishing during shutdown must not reopen a file
	// that would then never be flushed.
	if c.closed {
		return nil, fmt.Errorf("collector is closed")
	}

	if writer, ok := c.writers[name]; ok {
		return writer, nil
	}

	writer, err := rotate.New(rotate.Options{
		Dir:           c.dir,
		Name:          name,
		Ext:           "csv",
		MaxSize:       c.storage.MaxFileSize,
		RetainCount:   c.storage.RetainCount,
		Compression:   c.storage.Compression,
		Header:        csvHeader,
		FlushInterval: c.storage.FlushInterval,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics file for %s: %w", name, err)
	}

	c.writers[name] = writer
	return writer, nil
}
