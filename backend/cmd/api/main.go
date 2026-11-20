// Command api runs the HTTP backend of RAG Movie Picker.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/app"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/config"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/logger"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api stopped with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(cfg.LogLevel, os.Stdout)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("api starting", "addr", cfg.HTTPAddress, "log_level", cfg.LogLevel.String())

	application, err := app.New(ctx, cfg, log)
	if err != nil {
		return fmt.Errorf("build app: %w", err)
	}
	return application.Run(ctx)
}
