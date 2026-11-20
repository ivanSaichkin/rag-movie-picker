package config_test

import (
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/config"
)

const (
	testDatabaseURL = "postgres://app:app@127.0.0.1:5432/app?sslmode=disable"
	testJWTSecret   = "0123456789abcdef0123456789abcdef"
)

func TestLoadDefaults(t *testing.T) {
	unsetenv(t, "HTTP_ADDR")
	unsetenv(t, "LOG_LEVEL")
	unsetenv(t, "PG_MAX_CONNS")
	unsetenv(t, "ALLOWED_ORIGIN")
	unsetenv(t, "ACCESS_TOKEN_TTL")
	unsetenv(t, "REFRESH_TOKEN_TTL")
	unsetenv(t, "COOKIE_SECURE")
	unsetenv(t, "REDIS_ADDR")
	setRequired(t)

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
	if cfg.AllowedOrigin != "http://127.0.0.1:8080" {
		t.Fatalf("AllowedOrigin = %q", cfg.AllowedOrigin)
	}
	if cfg.AccessTokenTTL != 15*time.Minute {
		t.Fatalf("AccessTokenTTL = %v, want 15m", cfg.AccessTokenTTL)
	}
	if cfg.RefreshTokenTTL != 30*24*time.Hour {
		t.Fatalf("RefreshTokenTTL = %v, want 720h", cfg.RefreshTokenTTL)
	}
	if cfg.CookieSecure {
		t.Fatal("CookieSecure must default to false for local HTTP")
	}
	if cfg.Argon2.Iterations != 3 || cfg.Argon2.MemoryMiB != 64 || cfg.Argon2.Parallelism != 2 {
		t.Fatalf("Argon2 defaults = %+v", cfg.Argon2)
	}
	if cfg.RateLimit.AuthPerMinute != 10 || cfg.RateLimit.RegisterPerHour != 20 || cfg.RateLimit.RefreshPerMinute != 60 {
		t.Fatalf("RateLimit defaults = %+v", cfg.RateLimit)
	}
	if cfg.Redis.Addr != "" || cfg.Redis.DB != 0 || cfg.Redis.Timeout != time.Second {
		t.Fatalf("Redis defaults = %+v", cfg.Redis)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	unsetenv(t, "DATABASE_URL")
	t.Setenv("JWT_SECRET", testJWTSecret)

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() without DATABASE_URL must fail")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("error %q must mention DATABASE_URL", err)
	}
}

func TestLoadRequiresJWTSecret(t *testing.T) {
	t.Setenv("DATABASE_URL", testDatabaseURL)

	for _, secret := range []string{"", "short", strings.Repeat("a", 31)} {
		t.Run(secret, func(t *testing.T) {
			unsetenv(t, "JWT_SECRET")
			if secret != "" {
				t.Setenv("JWT_SECRET", secret)
			}

			_, err := config.Load()
			if err == nil {
				t.Fatal("Load() with a missing or short JWT_SECRET must fail")
			}
			if !strings.Contains(err.Error(), "JWT_SECRET") {
				t.Fatalf("error %q must mention JWT_SECRET", err)
			}
		})
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("HTTP_ADDR", "0.0.0.0:9000")
	t.Setenv("LOG_LEVEL", "WARN")
	t.Setenv("PG_MAX_CONNS", "25")
	t.Setenv("ALLOWED_ORIGIN", "https://movies.example.com")
	t.Setenv("ACCESS_TOKEN_TTL", "120")
	t.Setenv("REFRESH_TOKEN_TTL", "3600")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("ARGON2_ITERATIONS", "5")
	t.Setenv("ARGON2_MEMORY_MIB", "128")
	t.Setenv("ARGON2_PARALLELISM", "4")
	t.Setenv("RATE_LIMIT_AUTH_PER_MINUTE", "3")
	t.Setenv("RATE_LIMIT_REGISTER_PER_HOUR", "4")
	t.Setenv("RATE_LIMIT_REFRESH_PER_MINUTE", "5")
	t.Setenv("REDIS_ADDR", "127.0.0.1:6379")
	t.Setenv("REDIS_PASSWORD", "secret")
	t.Setenv("REDIS_DB", "2")
	t.Setenv("REDIS_TIMEOUT_SECONDS", "5")
	setRequired(t)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTPAddr != "0.0.0.0:9000" || cfg.LogLevel != slog.LevelWarn || cfg.PGMaxConns != 25 {
		t.Fatalf("basic overrides = %+v", cfg)
	}
	if cfg.AllowedOrigin != "https://movies.example.com" {
		t.Fatalf("AllowedOrigin = %q", cfg.AllowedOrigin)
	}
	if cfg.AccessTokenTTL != 120*time.Second || cfg.RefreshTokenTTL != time.Hour {
		t.Fatalf("TTLs = %v, %v", cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	}
	if !cfg.CookieSecure {
		t.Fatal("CookieSecure must be true")
	}
	if cfg.Argon2 != (config.Argon2Config{Iterations: 5, MemoryMiB: 128, Parallelism: 4}) {
		t.Fatalf("Argon2 = %+v", cfg.Argon2)
	}
	if cfg.RateLimit != (config.RateLimitConfig{AuthPerMinute: 3, RegisterPerHour: 4, RefreshPerMinute: 5}) {
		t.Fatalf("RateLimit = %+v", cfg.RateLimit)
	}
	if cfg.Redis != (config.RedisConfig{Addr: "127.0.0.1:6379", Password: "secret", DB: 2, Timeout: 5 * time.Second}) {
		t.Fatalf("Redis = %+v", cfg.Redis)
	}
}

func TestLoadAcceptsAllInterfacePort(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":8080")
	setRequired(t)

	if _, err := config.Load(); err != nil {
		t.Fatalf("Load() with empty host must succeed: %v", err)
	}
}

func TestLoadInvalidHTTPAddr(t *testing.T) {
	for _, addr := range []string{"", "127.0.0.1", "127.0.0.1:abc", "127.0.0.1:0", "127.0.0.1:70000"} {
		t.Run(addr, func(t *testing.T) {
			t.Setenv("HTTP_ADDR", addr)
			setRequired(t)

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

func TestLoadInvalidAllowedOrigin(t *testing.T) {
	for _, origin := range []string{"", "example.com", "ftp://example.com", "http://example.com/path", "http://example.com?q=1"} {
		t.Run(origin, func(t *testing.T) {
			t.Setenv("ALLOWED_ORIGIN", origin)
			setRequired(t)

			_, err := config.Load()
			if err == nil {
				t.Fatalf("Load() with ALLOWED_ORIGIN=%q must fail", origin)
			}
			if !strings.Contains(err.Error(), "ALLOWED_ORIGIN") {
				t.Fatalf("error %q must mention ALLOWED_ORIGIN", err)
			}
		})
	}
}

func TestLoadInvalidPGMaxConns(t *testing.T) {
	for _, raw := range []string{"", "zero", "0", "-1", "101"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv("PG_MAX_CONNS", raw)
			setRequired(t)

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

func TestLoadInvalidTTLs(t *testing.T) {
	for _, testCase := range []struct {
		key string
		raw string
	}{
		{"ACCESS_TOKEN_TTL", "0"},
		{"ACCESS_TOKEN_TTL", "abc"},
		{"ACCESS_TOKEN_TTL", "90000"},
		{"REFRESH_TOKEN_TTL", "-1"},
		{"REFRESH_TOKEN_TTL", "999999999"},
	} {
		t.Run(testCase.key+"="+testCase.raw, func(t *testing.T) {
			t.Setenv(testCase.key, testCase.raw)
			setRequired(t)

			_, err := config.Load()
			if err == nil {
				t.Fatalf("Load() with %s=%q must fail", testCase.key, testCase.raw)
			}
			if !strings.Contains(err.Error(), testCase.key) {
				t.Fatalf("error %q must mention %s", err, testCase.key)
			}
		})
	}
}

func TestLoadInvalidArgon2(t *testing.T) {
	for _, testCase := range []struct {
		key string
		raw string
	}{
		{"ARGON2_ITERATIONS", "0"},
		{"ARGON2_ITERATIONS", "21"},
		{"ARGON2_MEMORY_MIB", "4"},
		{"ARGON2_MEMORY_MIB", "2048"},
		{"ARGON2_PARALLELISM", "0"},
		{"ARGON2_PARALLELISM", "17"},
	} {
		t.Run(testCase.key+"="+testCase.raw, func(t *testing.T) {
			t.Setenv(testCase.key, testCase.raw)
			setRequired(t)

			_, err := config.Load()
			if err == nil {
				t.Fatalf("Load() with %s=%q must fail", testCase.key, testCase.raw)
			}
			if !strings.Contains(err.Error(), testCase.key) {
				t.Fatalf("error %q must mention %s", err, testCase.key)
			}
		})
	}
}

func TestLoadInvalidRateLimits(t *testing.T) {
	for _, key := range []string{"RATE_LIMIT_AUTH_PER_MINUTE", "RATE_LIMIT_REGISTER_PER_HOUR", "RATE_LIMIT_REFRESH_PER_MINUTE"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, "0")
			setRequired(t)

			_, err := config.Load()
			if err == nil {
				t.Fatalf("Load() with %s=0 must fail", key)
			}
			if !strings.Contains(err.Error(), key) {
				t.Fatalf("error %q must mention %s", err, key)
			}
		})
	}
}

func TestLoadInvalidCookieSecure(t *testing.T) {
	t.Setenv("COOKIE_SECURE", "sometimes")
	setRequired(t)

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() with a non-boolean COOKIE_SECURE must fail")
	}
	if !strings.Contains(err.Error(), "COOKIE_SECURE") {
		t.Fatalf("error %q must mention COOKIE_SECURE", err)
	}
}

func TestLoadInvalidRedisDB(t *testing.T) {
	t.Setenv("REDIS_DB", "16")
	setRequired(t)

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() with REDIS_DB=16 must fail")
	}
	if !strings.Contains(err.Error(), "REDIS_DB") {
		t.Fatalf("error %q must mention REDIS_DB", err)
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
			setRequired(t)

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
	setRequired(t)

	_, err := config.Load()
	if err == nil {
		t.Fatal("Load() with unknown LOG_LEVEL must fail")
	}
	if !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Fatalf("error %q must mention LOG_LEVEL", err)
	}
}

func setRequired(t *testing.T) {
	t.Helper()

	t.Setenv("DATABASE_URL", testDatabaseURL)
	t.Setenv("JWT_SECRET", testJWTSecret)
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
