package logger

import (
	"log/slog"
	"os"
)

func NewLogger() *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: slog.LevelDebug, // change to LevelInfo in production
	}

	handler := slog.NewJSONHandler(os.Stdout, opts)

	return slog.New(handler)
}
