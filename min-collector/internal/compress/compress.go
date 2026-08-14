// Package compress compresses rotated files. Three formats are supported so
// the trade off between speed and size can be chosen per deployment.
package compress

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/ulikunitz/xz"
	"gopkg.in/yaml.v3"
)

// Format is the compression applied to a rotated file.
type Format string

const (
	// None keeps rotated files uncompressed.
	None Format = "none"
	// Gzip is the most portable, every system can read it with zcat.
	Gzip Format = "gzip"
	// Zstd is the default, close to xz in size at gzip like speed.
	Zstd Format = "zstd"
	// Xz produces the smallest files but costs the most CPU and memory.
	Xz Format = "xz"
)

// Default is used when nothing is configured.
const Default = Zstd

// Formats lists every supported value, in the order shown to the user.
var Formats = []Format{Zstd, Gzip, Xz, None}

// Names returns the supported values as a comma separated list.
func Names() string {
	names := make([]string, 0, len(Formats))
	for _, format := range Formats {
		names = append(names, string(format))
	}
	return strings.Join(names, ", ")
}

// Parse converts a user supplied value into a Format. It also accepts the
// booleans the flag used to be, true meaning the default format.
func Parse(value string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(Zstd), "zst":
		return Zstd, nil
	case string(Gzip), "gz":
		return Gzip, nil
	case string(Xz):
		return Xz, nil
	case string(None), "off", "false", "no", "":
		return None, nil
	case "true", "yes", "on":
		return Default, nil
	default:
		return "", fmt.Errorf("unknown compression %q, use one of: %s", value, Names())
	}
}

// UnmarshalYAML accepts both a name and a boolean, so "compress: true" in a
// configuration file keeps working and means the default format.
func (f *Format) UnmarshalYAML(node *yaml.Node) error {
	var value string
	if err := node.Decode(&value); err != nil {
		return fmt.Errorf("compression must be one of: %s", Names())
	}

	parsed, err := Parse(value)
	if err != nil {
		return err
	}

	*f = parsed
	return nil
}

// Enabled reports whether rotated files are compressed.
func (f Format) Enabled() bool { return f != None && f != "" }

// Ext is the extension added to a compressed file.
func (f Format) Ext() string {
	switch f {
	case Gzip:
		return ".gz"
	case Zstd:
		return ".zst"
	case Xz:
		return ".xz"
	default:
		return ""
	}
}

// Tool is the command that reads a file in this format, shown to the user.
func (f Format) Tool() string {
	switch f {
	case Gzip:
		return "zcat"
	case Zstd:
		return "zstdcat"
	case Xz:
		return "xzcat"
	default:
		return "cat"
	}
}

// File compresses a file in place and removes the original. The compressed
// file is named after the original plus the format extension.
func File(path string, format Format) (string, error) {
	if !format.Enabled() {
		return path, nil
	}

	src, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() { _ = src.Close() }()

	dstPath := path + format.Ext()
	dst, err := os.Create(dstPath)
	if err != nil {
		return "", fmt.Errorf("failed to create compressed file: %w", err)
	}

	if err := copyCompressed(dst, src, format); err != nil {
		_ = dst.Close()
		_ = os.Remove(dstPath)
		return "", err
	}

	if err := dst.Close(); err != nil {
		_ = os.Remove(dstPath)
		return "", fmt.Errorf("failed to close compressed file: %w", err)
	}

	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("failed to remove uncompressed file: %w", err)
	}
	return dstPath, nil
}

// copyCompressed streams src into dst through the encoder of the format.
func copyCompressed(dst io.Writer, src io.Reader, format Format) error {
	writer, err := newWriter(dst, format)
	if err != nil {
		return err
	}

	if _, err := io.Copy(writer, src); err != nil {
		_ = writer.Close()
		return fmt.Errorf("failed to compress data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to finish %s stream: %w", format, err)
	}
	return nil
}

func newWriter(dst io.Writer, format Format) (io.WriteCloser, error) {
	switch format {
	case Gzip:
		return gzip.NewWriter(dst), nil
	case Zstd:
		writer, err := zstd.NewWriter(dst, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			return nil, fmt.Errorf("failed to create zstd writer: %w", err)
		}
		return writer, nil
	case Xz:
		writer, err := xz.NewWriter(dst)
		if err != nil {
			return nil, fmt.Errorf("failed to create xz writer: %w", err)
		}
		return writer, nil
	default:
		return nil, fmt.Errorf("unknown compression %q", format)
	}
}
