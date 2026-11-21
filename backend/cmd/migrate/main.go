// Command migrate applies the PostgreSQL migrations embedded into the binary.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/config"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/logger"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/persistence/postgres"
)

const usage = "usage: migrate up | down <steps> | version"

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("migrate failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New(usage)
	}

	databaseURL, err := config.LoadDatabaseURL()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	level, err := config.LoadLogLevel()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(level, os.Stdout)
	slog.SetDefault(log)

	switch args[0] {
	case "up":
		if err := postgres.Up(databaseURL); err != nil {
			return err
		}
		version, dirty, err := postgres.Version(databaseURL)
		if err != nil {
			return err
		}
		log.Info("migrations applied", "version", version, "dirty", dirty)
		return nil
	case "down":
		if len(args) != 2 {
			return fmt.Errorf("%s: down needs the number of steps", usage)
		}
		steps, err := strconv.Atoi(args[1])
		if err != nil || steps <= 0 {
			return fmt.Errorf("invalid steps %q, expected a positive integer", args[1])
		}
		if err := postgres.Down(databaseURL, steps); err != nil {
			return err
		}
		version, dirty, err := postgres.Version(databaseURL)
		if err != nil {
			return err
		}
		log.Info("migrations rolled back", "steps", steps, "version", version, "dirty", dirty)
		return nil
	case "version":
		version, dirty, err := postgres.Version(databaseURL)
		if err != nil {
			return err
		}
		fmt.Printf("version: %d\ndirty: %t\n", version, dirty)
		return nil
	default:
		return fmt.Errorf("%s: unknown command %q", usage, args[0])
	}
}
