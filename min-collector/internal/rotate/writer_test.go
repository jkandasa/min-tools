package rotate

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"

	"github.com/jkandasa/min-tools/min-collector/internal/compress"
)

func newTestWriter(t *testing.T, opts Options) *Writer {
	t.Helper()

	if opts.Dir == "" {
		opts.Dir = t.TempDir()
	}
	if opts.Name == "" {
		opts.Name = "test"
	}
	if opts.Ext == "" {
		opts.Ext = "ndjson"
	}
	if opts.MaxSize == 0 {
		opts.MaxSize = 1024
	}

	writer, err := New(opts)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	return writer
}

func TestWriteAndFlush(t *testing.T) {
	dir := t.TempDir()
	writer := newTestWriter(t, Options{Dir: dir})

	for range 3 {
		if err := writer.WriteLine([]byte(`{"a":1}`)); err != nil {
			t.Fatalf("write failed: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "test.ndjson"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if got := strings.Count(string(data), "\n"); got != 3 {
		t.Errorf("expected 3 lines, got %d", got)
	}
	if writer.Records() != 3 {
		t.Errorf("expected 3 records, got %d", writer.Records())
	}
}

func TestRotationCompressesAndRetains(t *testing.T) {
	for _, format := range compress.Formats {
		t.Run(string(format), func(t *testing.T) {
			dir := t.TempDir()
			writer := newTestWriter(t, Options{Dir: dir, MaxSize: 200, RetainCount: 2, Compression: format})

			record := bytes.Repeat([]byte("x"), 100)
			for range 10 {
				if err := writer.WriteLine(record); err != nil {
					t.Fatalf("write failed: %v", err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatalf("close failed: %v", err)
			}

			rotated, err := filepath.Glob(filepath.Join(dir, "test_*.ndjson"+format.Ext()))
			if err != nil {
				t.Fatalf("glob failed: %v", err)
			}
			if len(rotated) != 2 {
				t.Fatalf("expected 2 retained files, got %d", len(rotated))
			}

			content, err := readMaybeCompressed(rotated[0], format)
			if err != nil {
				t.Fatalf("failed to read rotated file: %v", err)
			}
			if !bytes.Contains(content, record) {
				t.Error("rotated file does not contain the written record")
			}
		})
	}
}

// readMaybeCompressed reads a rotated file back through its decoder.
func readMaybeCompressed(path string, format compress.Format) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var reader io.Reader = file
	switch format {
	case compress.Gzip:
		gz, err := gzip.NewReader(file)
		if err != nil {
			return nil, err
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	case compress.Zstd:
		zr, err := zstd.NewReader(file)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		reader = zr
	case compress.Xz:
		xr, err := xz.NewReader(file)
		if err != nil {
			return nil, err
		}
		reader = xr
	}

	return io.ReadAll(reader)
}

func TestHeaderWrittenOncePerFile(t *testing.T) {
	dir := t.TempDir()
	header := "timestamp,value\n"

	writer := newTestWriter(t, Options{Dir: dir, Ext: "csv", Header: header, MaxSize: 60})
	for range 5 {
		if err := writer.WriteLine([]byte("2026-01-01T00:00:00Z,1")); err != nil {
			t.Fatalf("write failed: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	// Reopening must append to the active file without repeating the header.
	writer = newTestWriter(t, Options{Dir: dir, Ext: "csv", Header: header, MaxSize: 1 << 20})
	if err := writer.WriteLine([]byte("2026-01-01T00:00:01Z,2")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "test.csv"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if got := strings.Count(string(data), header); got != 1 {
		t.Errorf("expected exactly 1 header in the active file, got %d", got)
	}

	rotated, err := filepath.Glob(filepath.Join(dir, "test_*.csv*"))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(rotated) == 0 {
		t.Fatal("expected at least one rotated file")
	}
	for _, path := range rotated {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read %s: %v", path, err)
		}
		if !bytes.HasPrefix(content, []byte(header)) {
			t.Errorf("rotated file %s is missing the header", filepath.Base(path))
		}
	}
}

func TestRecordLargerThanMaxSizeIsWritten(t *testing.T) {
	dir := t.TempDir()
	writer := newTestWriter(t, Options{Dir: dir, MaxSize: 10})

	large := bytes.Repeat([]byte("y"), 100)
	if err := writer.WriteLine(large); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := writer.WriteLine(large); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "test.ndjson"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if !bytes.Contains(data, large) {
		t.Error("oversized record was not written")
	}
}

func TestWriteAfterCloseFails(t *testing.T) {
	writer := newTestWriter(t, Options{})
	if err := writer.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if err := writer.WriteLine([]byte("late")); err == nil {
		t.Error("expected an error when writing to a closed writer")
	}
}
