// Package logx wraps the zap logger used across min-collector and exposes a
// small printf style API on top of it.
package logx

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Log formats supported by Init.
const (
	FormatConsole = "console"
	FormatJSON    = "json"
)

// Log levels supported by Init, in increasing order of severity.
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"
)

// Levels lists every supported level, in the order shown to the user.
var Levels = []string{LevelDebug, LevelInfo, LevelWarn, LevelError}

// Formats lists every supported log format.
var Formats = []string{FormatConsole, FormatJSON}

var (
	mu      sync.RWMutex
	base    = zap.NewNop()
	sugared = base.Sugar()
	helper  = base.Sugar()
	debug   bool
)

// ParseLevel converts a level name into a zap level.
func ParseLevel(name string) (zapcore.Level, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case LevelDebug:
		return zapcore.DebugLevel, nil
	case LevelInfo, "":
		return zapcore.InfoLevel, nil
	case LevelWarn, "warning":
		return zapcore.WarnLevel, nil
	case LevelError:
		return zapcore.ErrorLevel, nil
	default:
		return 0, fmt.Errorf("unknown log level %q, use one of: %s", name, strings.Join(Levels, ", "))
	}
}

// ParseFormat validates a log format name.
func ParseFormat(name string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case FormatConsole, "":
		return FormatConsole, nil
	case FormatJSON:
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("unknown log format %q, use one of: %s", name, strings.Join(Formats, ", "))
	}
}

// Init builds the process logger. Console is human readable, json is meant for
// log shippers. An invalid level or format falls back to the default, Parse
// them first to report the problem to the user. It is safe to call more than
// once.
func Init(levelName, format string) {
	level, err := ParseLevel(levelName)
	if err != nil {
		level = zapcore.InfoLevel
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder
	encoderConfig.TimeKey = "time"
	encoderConfig.MessageKey = "msg"
	encoderConfig.CallerKey = "caller"

	var encoder zapcore.Encoder
	if strings.EqualFold(format, FormatJSON) {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	// Logs go to stderr so collected data can be piped on stdout if needed.
	core := zapcore.NewCore(encoder, zapcore.Lock(os.Stderr), level)
	logger := zap.New(core, zap.AddCaller(), zap.ErrorOutput(zapcore.Lock(os.Stderr)))

	mu.Lock()
	defer mu.Unlock()
	base = logger
	sugared = logger.Sugar()
	// The printf helpers below add one frame, without skipping it every line
	// would be reported as coming from this file.
	helper = logger.WithOptions(zap.AddCallerSkip(1)).Sugar()
	debug = level == zapcore.DebugLevel
}

// L returns the underlying zap logger, for callers that want structured fields.
func L() *zap.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return base
}

// S returns the underlying sugared logger.
func S() *zap.SugaredLogger {
	mu.RLock()
	defer mu.RUnlock()
	return sugared
}

// DebugEnabled reports whether debug logging is on.
func DebugEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return debug
}

// StdLogger returns a *log.Logger that forwards to zap at error level, for
// APIs such as http.Server.ErrorLog.
func StdLogger() *log.Logger {
	logger, err := zap.NewStdLogAt(L(), zapcore.ErrorLevel)
	if err != nil {
		return log.New(os.Stderr, "", log.LstdFlags)
	}
	return logger
}

// helperS returns the sugared logger that reports the caller of the printf
// helpers rather than this file.
func helperS() *zap.SugaredLogger {
	mu.RLock()
	defer mu.RUnlock()
	return helper
}

// Debugf logs at debug level, with the file and line of the caller.
func Debugf(format string, v ...any) { helperS().Debugf(format, v...) }

// Infof logs at info level, with the file and line of the caller.
func Infof(format string, v ...any) { helperS().Infof(format, v...) }

// Warnf logs at warn level, with the file and line of the caller.
func Warnf(format string, v ...any) { helperS().Warnf(format, v...) }

// Errorf logs at error level, with the file and line of the caller.
func Errorf(format string, v ...any) { helperS().Errorf(format, v...) }

// Fatalf logs at fatal level and exits with a non-zero status.
func Fatalf(format string, v ...any) {
	Sync()
	helperS().Fatalf(format, v...)
}

// Print writes a raw line to stdout without any decoration, used for the
// startup banner.
func Print(msg string) {
	if _, err := fmt.Fprintln(os.Stdout, msg); err != nil {
		Errorf("failed to write to stdout: %v", err)
	}
}

// Sync flushes buffered log entries.
func Sync() {
	// Syncing stderr returns an error on some platforms, it is not actionable.
	_ = L().Sync()
}
