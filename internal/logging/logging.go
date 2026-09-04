package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// Setup configures process-wide slog logging to a file (never the TUI).
func Setup(logPath string, debug bool) (*slog.Logger, *os.File, error) {
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, fmt.Errorf("open log file: %w", err)
	}

	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: level})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger, f, nil
}

// WALogger adapts slog to whatsmeow's logger interface without writing to stdout.
type WALogger struct {
	mod    string
	logger *slog.Logger
	debug  bool
}

// NewWALogger returns a whatsmeow-compatible logger backed by slog.
func NewWALogger(logger *slog.Logger, module string, debug bool) *WALogger {
	return &WALogger{
		mod:    module,
		logger: logger.With("module", module),
		debug:  debug,
	}
}

func (l *WALogger) Warnf(msg string, args ...any)  { l.logger.Warn(fmt.Sprintf(msg, args...)) }
func (l *WALogger) Errorf(msg string, args ...any) { l.logger.Error(fmt.Sprintf(msg, args...)) }
func (l *WALogger) Infof(msg string, args ...any)  { l.logger.Info(fmt.Sprintf(msg, args...)) }
func (l *WALogger) Debugf(msg string, args ...any) {
	if l.debug {
		l.logger.Debug(fmt.Sprintf(msg, args...))
	}
}

func (l *WALogger) Sub(module string) waLog.Logger {
	name := l.mod
	if name != "" {
		name = name + "/" + module
	} else {
		name = module
	}
	return NewWALogger(l.logger, name, l.debug)
}

// Discard is a no-op writer helper for tests.
var Discard io.Writer = io.Discard

// ParseLevel is reserved for future config parsing.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
