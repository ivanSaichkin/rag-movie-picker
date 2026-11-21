// Package logger builds the application logger.
package logger

import (
	"io"
	"log/slog"
)

// New returns a JSON logger writing to out. Structured logs keep the service
// output machine-readable for the deployment that collects it.
func New(level slog.Level, out io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: level}))
}
