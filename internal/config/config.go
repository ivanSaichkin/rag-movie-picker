// Package config loads the service configuration from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
)

const (
	defaultHTTPAddr = "127.0.0.1:8080"
	defaultLogLevel = slog.LevelInfo
)

// Config is the runtime configuration of the API service.
type Config struct {
	HTTPAddr string
	LogLevel slog.Level
}

// Load reads the configuration from the environment. It contains only the
// settings required by the current stage; new variables are added together
// with their consumers so that no configuration stays unused.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr: defaultHTTPAddr,
		LogLevel: defaultLogLevel,
	}

	if raw, ok := os.LookupEnv("HTTP_ADDR"); ok {
		cfg.HTTPAddr = strings.TrimSpace(raw)
	}
	if err := validateAddr(cfg.HTTPAddr); err != nil {
		return Config{}, fmt.Errorf("HTTP_ADDR: %w", err)
	}

	if raw, ok := os.LookupEnv("LOG_LEVEL"); ok {
		level, err := parseLogLevel(raw)
		if err != nil {
			return Config{}, err
		}
		cfg.LogLevel = level
	}

	return cfg, nil
}

// validateAddr accepts "host:port" and ":port"; the host stays optional so the
// service can listen on all interfaces inside a container.
func validateAddr(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("expected host:port, got %q: %w", addr, err)
	}
	if port == "" {
		return fmt.Errorf("missing port in %q", addr)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number <= 0 || number > 65535 {
		return fmt.Errorf("invalid port %q in %q", port, addr)
	}
	return nil
}

func parseLogLevel(raw string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("LOG_LEVEL: unknown level %q, expected debug, info, warn or error", raw)
	}
}
