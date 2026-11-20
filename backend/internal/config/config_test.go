package config_test

import (
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/config"
)

const testDatabaseURL = "postgres://app:app@127.0.0.1:5432/app?sslmode=disable"

func TestLoadDefaults(t *testing.T) {
	unsetenv(t, "HTTP_ADDR")
	unsetenv(t, "LOG_LEVEL")
	unsetenv(t, "PG_MAX_CONNS")
	t.Setenv("DATABASE_URL", testDatabaseURL)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != "127.0.0.1:8080" {
		t.Fatalf("HTTPAddr = %q, want 127.0.0.1:8080", cfg.HTTPAddr)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("LogLevel = %v, want info", cfg.LogLevel)
	}
	if cfg.DatabaseURL != testDatabaseURL {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
	if cfg.PGMaxConns != 10 {
		t.Fatalf("PGMaxConns = %d, want 10", cfg.PGMaxConns)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	unsetenv(t, "DATABASE_URL")

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() without DATABASE_URL must fail")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("error %q must mention DATABASE_URL", err)
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("HTTP_ADDR", "0.0.0.0:9000")
	t.Setenv("LOG_LEVEL", "WARN")
	t.Setenv("DATABASE_URL", testDatabaseURL)
	t.Setenv("PG_MAX_CONNS", "25")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != "0.0.0.0:9000" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.LogLevel != slog.LevelWarn {
		t.Fatalf("LogLevel = %v, want warn", cfg.LogLevel)
	}
	if cfg.PGMaxConns != 25 {
		t.Fatalf("PGMaxConns = %d, want 25", cfg.PGMaxConns)
	}
}

func TestLoadAcceptsAllInterfacePort(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":8080")
	t.Setenv("DATABASE_URL", testDatabaseURL)

	if _, err := config.Load(); err != nil {
		t.Fatalf("Load() with empty host must succeed: %v", err)
	}
}

func TestLoadInvalidHTTPAddr(t *testing.T) {
	for _, addr := range []string{"", "127.0.0.1", "127.0.0.1:abc", "127.0.0.1:0", "127.0.0.1:70000"} {
		t.Run(addr, func(t *testing.T) {
			t.Setenv("HTTP_ADDR", addr)
			t.Setenv("DATABASE_URL", testDatabaseURL)

			_, err := config.Load()
			if err == nil {
				t.Fatalf("Load() with HTTP_ADDR=%q must fail", addr)
			}
			if !strings.Contains(err.Error(), "HTTP_ADDR") {
				t.Fatalf("error %q must mention HTTP_ADDR", err)
			}
		})
	}
}

func TestLoadInvalidPGMaxConns(t *testing.T) {
	for _, raw := range []string{"", "zero", "0", "-1", "101"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("DATABASE_URL", testDatabaseURL)
			t.Setenv("PG_MAX_CONNS", raw)

			_, err := config.Load()
			if err == nil {
				t.Fatalf("Load() with PG_MAX_CONNS=%q must fail", raw)
			}
			if !strings.Contains(err.Error(), "PG_MAX_CONNS") {
				t.Fatalf("error %q must mention PG_MAX_CONNS", err)
			}
		})
	}
}

func TestLoadLogLevels(t *testing.T) {
	for raw, want := range map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"INFO":    slog.LevelInfo,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
	} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("LOG_LEVEL", raw)
			t.Setenv("DATABASE_URL", testDatabaseURL)

			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.LogLevel != want {
				t.Fatalf("LogLevel = %v, want %v", cfg.LogLevel, want)
			}
		})
	}
}

func TestLoadInvalidLogLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "verbose")
	t.Setenv("DATABASE_URL", testDatabaseURL)

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() with unknown LOG_LEVEL must fail")
	}
	if !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Fatalf("error %q must mention LOG_LEVEL", err)
	}
}

// unsetenv removes a variable for the test and restores it afterwards, so the
// defaults are checked even when the developer already exported settings.
func unsetenv(t *testing.T, key string) {
	t.Helper()

	previous, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if !existed {
			return
		}
		if err := os.Setenv(key, previous); err != nil {
			t.Fatalf("restore %s: %v", key, err)
		}
	})
}
