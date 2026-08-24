// Package logger wires a single structured logger (slog) for the whole binary.
//
// Why slog: it's the stdlib answer to logrus/zap, plays well with JSON
// pipelines, and removes one transitive dependency. We expose a *slog.Logger so
// callers can attach context.
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/iatneh/ip2loc/internal/config"
)

// New constructs the root logger from config.
func New(cfg config.LogConfig) (*slog.Logger, error) {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: false,
	}

	handler, err := newHandler(cfg.Format, cfg.Output, opts)
	if err != nil {
		return nil, err
	}
	return slog.New(handler), nil
}

// newHandler builds the slog handler per format+output choice.
func newHandler(format, output string, opts *slog.HandlerOptions) (slog.Handler, error) {
	writer, err := openOutput(output)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(format) {
	case "json":
		return slog.NewJSONHandler(writer, opts), nil
	default:
		return slog.NewTextHandler(writer, opts), nil
	}
}

// openOutput returns an io.Writer for the configured output.
// Multiple comma-separated outputs are supported.
func openOutput(output string) (io.Writer, error) {
	if output == "" {
		return os.Stdout, nil
	}
	parts := strings.Split(output, ",")
	writers := make([]io.Writer, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		switch strings.ToLower(p) {
		case "stdout":
			writers = append(writers, os.Stdout)
		case "stderr":
			writers = append(writers, os.Stderr)
		default:
			f, err := openLogFile(p)
			if err != nil {
				return nil, err
			}
			writers = append(writers, f)
		}
	}
	if len(writers) == 0 {
		return os.Stdout, nil
	}
	if len(writers) == 1 {
		return writers[0], nil
	}
	return io.MultiWriter(writers...), nil
}

func openLogFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir for log %q: %w", path, err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file %q: %w", path, err)
	}
	return f, nil
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level %q", s)
	}
}

// SetDefault swaps slog's default logger so packages that call slog.Info
// without a logger still go through our pipeline.
func SetDefault(l *slog.Logger) {
	slog.SetDefault(l)
}
