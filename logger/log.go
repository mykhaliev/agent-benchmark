package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/lmittmann/tint"
)

var Logger *slog.Logger

const (
	FilePermission = 0644
)

func SetupLogger(w io.Writer, verbose bool) {
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}

	opts := &tint.Options{
		Level:      logLevel,
		TimeFormat: "2006-01-02 15:04:05",
	}

	handler := tint.NewHandler(w, opts)

	Logger = slog.New(handler)
}

func SetupLogWriter(logPath string) (io.Writer, *os.File, error) {
	if logPath == "" {
		return os.Stdout, nil, nil
	}

	// Ensure log directory exists
	logDir := filepath.Dir(logPath)
	if logDir != "." && logDir != "" {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, nil, fmt.Errorf("failed to create log directory: %w", err)
		}
	}

	// Open log file
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, FilePermission)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open log file: %w", err)
	}

	// Write to both stdout and file
	multiWriter := io.MultiWriter(os.Stdout, logFile)
	return multiWriter, logFile, nil
}
