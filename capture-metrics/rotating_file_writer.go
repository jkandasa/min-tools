package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type RotatingFileWriter struct {
	dir          string
	baseFilename string
	maxSize      int64
	retainCount  int
	currentFile  *os.File
	currentSize  int64
	mu           sync.Mutex
}

func NewRotatingFileWriter(dir, baseFilename string, maxSize int64, retainCount int) *RotatingFileWriter {
	return &RotatingFileWriter{
		dir:          dir,
		baseFilename: sanitizeFilename(baseFilename),
		maxSize:      maxSize,
		retainCount:  retainCount,
	}
}

func sanitizeFilename(name string) string {
	// Replace problematic characters with underscores
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
	)
	return replacer.Replace(name)
}

func (rfw *RotatingFileWriter) Write(data []byte) error {
	rfw.mu.Lock()
	defer rfw.mu.Unlock()

	// Check if we need to open a new file or rotate
	if rfw.currentFile == nil {
		if err := rfw.openNewFile(); err != nil {
			return err
		}
	}

	// Check if we need to rotate based on size
	if rfw.currentSize+int64(len(data)) > rfw.maxSize {
		if err := rfw.rotate(); err != nil {
			return err
		}
	}

	// Write data
	n, err := rfw.currentFile.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}

	rfw.currentSize += int64(n)
	return nil
}

func (rfw *RotatingFileWriter) openNewFile() error {
	filename := filepath.Join(rfw.dir, fmt.Sprintf("%s.csv", rfw.baseFilename))

	// Check if file exists and get its size
	var fileSize int64
	if fileInfo, err := os.Stat(filename); err == nil {
		fileSize = fileInfo.Size()
	}

	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filename, err)
	}

	rfw.currentFile = file
	rfw.currentSize = fileSize
	return nil
}

func (rfw *RotatingFileWriter) rotate() error {
	// Close current file
	if rfw.currentFile != nil {
		if err := rfw.currentFile.Close(); err != nil {
			return fmt.Errorf("failed to close current file: %w", err)
		}
		rfw.currentFile = nil
	}

	// Generate rotation timestamp
	timestamp := time.Now().Format("20060102_150405")
	currentFilename := filepath.Join(rfw.dir, fmt.Sprintf("%s.csv", rfw.baseFilename))
	rotatedFilename := filepath.Join(rfw.dir, fmt.Sprintf("%s_%s.csv", rfw.baseFilename, timestamp))

	// Rename current file
	if err := os.Rename(currentFilename, rotatedFilename); err != nil {
		// If file doesn't exist, that's okay, we'll create a new one
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to rotate file: %w", err)
		}
	} else {
		// Compress the rotated file
		if err := rfw.compressFile(rotatedFilename); err != nil {
			log.Printf("Warning: failed to compress rotated file: %v", err)
			// Continue even if compression fails
		}
	}

	// Clean up old files if needed
	if err := rfw.cleanupOldFiles(); err != nil {
		// Log but don't fail on cleanup errors
		log.Printf("Warning: failed to cleanup old files: %v", err)
	}

	// Open new file
	return rfw.openNewFile()
}

// compressFile compresses a file using gzip and removes the original
func (rfw *RotatingFileWriter) compressFile(filename string) error {
	// Open source file
	srcFile, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	// Create destination file with .gz extension
	dstFilename := filename + ".gz"
	dstFile, err := os.Create(dstFilename)
	if err != nil {
		return fmt.Errorf("failed to create compressed file: %w", err)
	}
	defer dstFile.Close()

	// Create gzip writer
	gzWriter := gzip.NewWriter(dstFile)
	defer gzWriter.Close()

	// Copy and compress
	if _, err := io.Copy(gzWriter, srcFile); err != nil {
		return fmt.Errorf("failed to compress data: %w", err)
	}

	// Flush gzip writer
	if err := gzWriter.Close(); err != nil {
		return fmt.Errorf("failed to close gzip writer: %w", err)
	}

	// Close destination file
	if err := dstFile.Close(); err != nil {
		return fmt.Errorf("failed to close destination file: %w", err)
	}

	// Remove original file
	if err := os.Remove(filename); err != nil {
		return fmt.Errorf("failed to remove original file: %w", err)
	}

	DebugLog("Compressed and removed original file: %s -> %s", filename, dstFilename)
	return nil
}

func (rfw *RotatingFileWriter) cleanupOldFiles() error {
	// If retainCount is 0, keep all files
	if rfw.retainCount == 0 {
		return nil
	}

	// Find all rotated files for this metric (both .csv and .csv.gz)
	pattern := filepath.Join(rfw.dir, fmt.Sprintf("%s_*.csv*", rfw.baseFilename))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to glob files: %w", err)
	}

	// If we don't have more files than the retain count, nothing to do
	if len(matches) <= rfw.retainCount {
		return nil
	}

	// Sort files by modification time (oldest first)
	type fileInfo struct {
		path    string
		modTime time.Time
	}

	var files []fileInfo
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		files = append(files, fileInfo{
			path:    match,
			modTime: info.ModTime(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	// Delete oldest files to maintain retain count
	deleteCount := len(files) - rfw.retainCount
	for i := 0; i < deleteCount; i++ {
		if err := os.Remove(files[i].path); err != nil {
			log.Printf("Warning: failed to remove old file %s: %v", files[i].path, err)
		} else {
			DebugLog("Deleted old file: %s", files[i].path)
		}
	}

	return nil
}

func (rfw *RotatingFileWriter) Close() error {
	rfw.mu.Lock()
	defer rfw.mu.Unlock()

	if rfw.currentFile != nil {
		err := rfw.currentFile.Close()
		rfw.currentFile = nil
		return err
	}
	return nil
}

// GetCurrentFilePath returns the current file path
func (rfw *RotatingFileWriter) GetCurrentFilePath() string {
	rfw.mu.Lock()
	defer rfw.mu.Unlock()
	return filepath.Join(rfw.dir, fmt.Sprintf("%s.csv", rfw.baseFilename))
}
