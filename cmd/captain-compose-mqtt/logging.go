package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

func SetupLogger(cfg LogConfig) error {
	var level slog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var outputs []io.Writer
	outputs = append(outputs, os.Stdout)

	if cfg.FilePath != "" {
		logFile, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file: %w", err)
		}
		outputs = append(outputs, logFile)
	}

	writer := io.MultiWriter(outputs...)

	var handler slog.Handler
	switch strings.ToLower(cfg.Format) {
	case "json":
		handler = slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: level})
	default:
		handler = slog.NewTextHandler(writer, &slog.HandlerOptions{Level: level})
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	return nil
}
