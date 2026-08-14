// Package rotate implements a size based rotating file writer with optional
// gzip compression and retention. It is shared by every collected stream
// (metrics, logger, audit) so rotation behaves identically for all of them.
package rotate

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jkandasa/min-tools/min-collector/internal/compress"
	"github.com/jkandasa/min-tools/min-collector/internal/logx"
)

// rotationStampLayout is the suffix added to a rotated file.
const rotationStampLayout = "20060102_150405"

// rotationQueueSize is how many rotated files may wait for compression before
// a rotation starts to block.
const rotationQueueSize = 64

// Options configures a Writer.
type Options struct {
	// Dir is the directory holding the active and rotated files.
	Dir string
	// Name is the base file name, without extension.
	Name string
	// Ext is the file extension without the leading dot, for example "csv".
	Ext string
	// MaxSize is the size in bytes at which the active file is rotated.
	MaxSize int64
	// RetainCount is the number of rotated files to keep, 0 keeps everything.
	RetainCount int
	// Compression is applied to rotated files.
	Compression compress.Format
	// Header, when set, is written at the top of every new file.
	Header string
	// FlushInterval controls how often buffered data is flushed to disk.
	// Zero disables the background flush.
	FlushInterval time.Duration
}

// Writer appends records to a file, rotating, compressing and pruning it as
// configured. It is safe for concurrent use.
type Writer struct {
	opts Options

	mu          sync.Mutex
	file        *os.File
	buf         *bufio.Writer
	currentSize int64
	closed      bool

	// Rotated files are compressed and pruned by a single worker so the two
	// steps never race with each other.
	rotations  chan string
	workerDone chan struct{}

	// pendingMu guards pending, the set of rotated files not yet compressed.
	// Retention must not delete a file while it is still being processed.
	pendingMu sync.Mutex
	pending   map[string]struct{}

	stopFlush chan struct{}
	flushDone chan struct{}

	records atomic.Int64
	bytes   atomic.Int64
}

// New creates a Writer. The directory is created if missing.
func New(opts Options) (*Writer, error) {
	if opts.Ext == "" {
		opts.Ext = "log"
	}
	if opts.MaxSize <= 0 {
		return nil, fmt.Errorf("max size must be positive")
	}
	opts.Name = sanitizeName(opts.Name)

	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", opts.Dir, err)
	}

	w := &Writer{
		opts:       opts,
		rotations:  make(chan string, rotationQueueSize),
		workerDone: make(chan struct{}),
		pending:    make(map[string]struct{}),
	}
	if err := w.openLocked(); err != nil {
		return nil, err
	}
	go w.rotationWorker()

	if opts.FlushInterval > 0 {
		w.stopFlush = make(chan struct{})
		w.flushDone = make(chan struct{})
		go w.flushLoop(opts.FlushInterval)
	}
	return w, nil
}

// Path returns the path of the active file.
func (w *Writer) Path() string {
	return filepath.Join(w.opts.Dir, fmt.Sprintf("%s.%s", w.opts.Name, w.opts.Ext))
}

// Records returns the number of records written since start.
func (w *Writer) Records() int64 { return w.records.Load() }

// Bytes returns the number of bytes written since start.
func (w *Writer) Bytes() int64 { return w.bytes.Load() }

// Write appends data, rotating first when the active file would exceed the
// configured maximum size.
func (w *Writer) Write(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return fmt.Errorf("writer is closed")
	}

	// Rotate only when the file already holds data, so a single record larger
	// than the limit does not trigger an endless rotation loop.
	if w.currentSize > 0 && w.currentSize+int64(len(data)) > w.opts.MaxSize {
		if err := w.rotateLocked(); err != nil {
			return err
		}
	}

	n, err := w.buf.Write(data)
	w.currentSize += int64(n)
	w.bytes.Add(int64(n))
	if err != nil {
		return fmt.Errorf("failed to write to %s: %w", w.Path(), err)
	}
	w.records.Add(1)
	return nil
}

// WriteLine appends data followed by a newline.
func (w *Writer) WriteLine(data []byte) error {
	line := make([]byte, 0, len(data)+1)
	line = append(line, data...)
	line = append(line, '\n')
	return w.Write(line)
}

// Flush writes buffered data to the operating system.
func (w *Writer) Flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flushLocked()
}

func (w *Writer) flushLocked() error {
	if w.buf == nil {
		return nil
	}
	if err := w.buf.Flush(); err != nil {
		return fmt.Errorf("failed to flush %s: %w", w.Path(), err)
	}
	return nil
}

// Close flushes pending data, closes the active file and waits for any
// in-flight compression to finish.
func (w *Writer) Close() error {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil
	}
	w.closed = true
	w.mu.Unlock()

	if w.stopFlush != nil {
		close(w.stopFlush)
		<-w.flushDone
	}

	w.mu.Lock()
	err := w.closeFileLocked()
	w.mu.Unlock()

	// Let the worker finish compressing whatever is still queued.
	close(w.rotations)
	<-w.workerDone
	return err
}

func (w *Writer) flushLoop(interval time.Duration) {
	defer close(w.flushDone)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := w.Flush(); err != nil {
				logx.Errorf("%v", err)
			}
		case <-w.stopFlush:
			return
		}
	}
}

func (w *Writer) openLocked() error {
	path := w.Path()

	var size int64
	if info, err := os.Stat(path); err == nil {
		size = info.Size()
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", path, err)
	}

	w.file = file
	w.buf = bufio.NewWriterSize(file, 64*1024)
	w.currentSize = size

	// A header belongs at the top of every file, including rotated ones, but
	// must not be repeated when appending to an existing file after a restart.
	if w.opts.Header != "" && size == 0 {
		n, err := w.buf.WriteString(w.opts.Header)
		w.currentSize += int64(n)
		if err != nil {
			return fmt.Errorf("failed to write header to %s: %w", path, err)
		}
	}
	return nil
}

func (w *Writer) closeFileLocked() error {
	if w.file == nil {
		return nil
	}
	flushErr := w.flushLocked()
	closeErr := w.file.Close()
	w.file = nil
	w.buf = nil
	if flushErr != nil {
		return flushErr
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close file: %w", closeErr)
	}
	return nil
}

func (w *Writer) rotateLocked() error {
	if err := w.closeFileLocked(); err != nil {
		return err
	}

	active := w.Path()
	stamp := time.Now().Format(rotationStampLayout)
	rotated := filepath.Join(w.opts.Dir, fmt.Sprintf("%s_%s.%s", w.opts.Name, stamp, w.opts.Ext))

	// A collector restarted within the same second would otherwise clobber an
	// existing rotated file, compressed or not.
	for i := 1; fileExists(rotated) || fileExists(rotated+w.opts.Compression.Ext()); i++ {
		rotated = filepath.Join(w.opts.Dir, fmt.Sprintf("%s_%s-%d.%s", w.opts.Name, stamp, i, w.opts.Ext))
	}

	if err := os.Rename(active, rotated); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to rotate %s: %w", active, err)
		}
	} else {
		logx.Debugf("rotated %s -> %s", filepath.Base(active), filepath.Base(rotated))
		w.postRotate(rotated)
	}

	w.currentSize = 0
	return w.openLocked()
}

// postRotate hands the rotated file to the worker so a large rotation does not
// stall incoming records.
func (w *Writer) postRotate(rotated string) {
	w.pendingMu.Lock()
	w.pending[rotated] = struct{}{}
	w.pendingMu.Unlock()

	w.rotations <- rotated
}

// rotationWorker compresses and prunes rotated files one at a time.
func (w *Writer) rotationWorker() {
	defer close(w.workerDone)

	for rotated := range w.rotations {
		if w.opts.Compression.Enabled() {
			compressed, err := compress.File(rotated, w.opts.Compression)
			if err != nil {
				logx.Warnf("failed to compress %s: %v", rotated, err)
			} else {
				logx.Debugf("compressed %s -> %s", filepath.Base(rotated), filepath.Base(compressed))
			}
		}

		w.pendingMu.Lock()
		delete(w.pending, rotated)
		w.pendingMu.Unlock()

		if err := w.cleanup(); err != nil {
			logx.Warnf("failed to clean up old files: %v", err)
		}
	}
}

// isPending reports whether a file is still waiting to be compressed.
func (w *Writer) isPending(path string) bool {
	w.pendingMu.Lock()
	defer w.pendingMu.Unlock()

	_, ok := w.pending[path]
	return ok
}

func (w *Writer) cleanup() error {
	if w.opts.RetainCount <= 0 {
		return nil
	}

	pattern := filepath.Join(w.opts.Dir, fmt.Sprintf("%s_*.%s*", w.opts.Name, w.opts.Ext))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to list rotated files: %w", err)
	}
	if len(matches) <= w.opts.RetainCount {
		return nil
	}

	type entry struct {
		path    string
		modTime time.Time
	}

	files := make([]entry, 0, len(matches))
	for _, match := range matches {
		// A file still queued for compression must survive until it is done.
		if w.isPending(match) {
			continue
		}
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		files = append(files, entry{path: match, modTime: info.ModTime()})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	for i := 0; i < len(files)-w.opts.RetainCount; i++ {
		if err := os.Remove(files[i].path); err != nil {
			logx.Warnf("failed to remove old file %s: %v", files[i].path, err)
			continue
		}
		logx.Debugf("removed old file %s", filepath.Base(files[i].path))
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// sanitizeName strips characters that are awkward or unsafe in file names.
func sanitizeName(name string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		" ", "_",
	)
	return replacer.Replace(name)
}
