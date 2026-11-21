// Package config loads the service configuration from environment variables.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPAddr   = "127.0.0.1:8080"
	defaultLogLevel   = slog.LevelInfo
	defaultPGMaxConns = 10
	maxPGMaxConns     = 100

	defaultAllowedOrigin     = "http://127.0.0.1:8080"
	defaultAccessTokenTTL    = 15 * time.Minute
	defaultRefreshTokenTTL   = 30 * 24 * time.Hour
	defaultArgon2Iterations  = 3
	defaultArgon2MemoryMiB   = 64
	defaultArgon2Parallelism = 2

	defaultRateLimitAuthPerMinute    = 10
	defaultRateLimitRegisterPerHour  = 20
	defaultRateLimitRefreshPerMinute = 60
	defaultRAGGRPCAddress            = "127.0.0.1:50051"
	defaultRAGTimeout                = 30 * time.Second
	minJWTSecretLength               = 32
	redisDefaultDB                   = 0
	defaultRedisTimeout              = time.Second

	maxAccessTokenTTLSeconds  = 24 * 60 * 60
	maxRefreshTokenTTLSeconds = 365 * 24 * 60 * 60
	maxArgon2Iterations       = 20
	maxArgon2MemoryMiB        = 1024
	maxArgon2Parallelism      = 16
	maxRateLimitPerWindow     = 100000
	maxRAGTimeoutSeconds      = 120
)

// Argon2Config holds the password hashing cost parameters.
type Argon2Config struct {
	Iterations  uint32
	MemoryMiB   uint32
	Parallelism uint8
}

// RateLimitConfig holds the per-window request limits for authentication.
type RateLimitConfig struct {
	AuthPerMinute    int64
	RegisterPerHour  int64
	RefreshPerMinute int64
}

// RedisConfig describes the optional Redis connection. An empty address means
// that the in-memory implementation is used.
type RedisConfig struct {
	Address  string
	Password string
	Database int
	Timeout  time.Duration
}

// Config is the runtime configuration of the API service.
type Config struct {
	HTTPAddress            string
	LogLevel               slog.Level
	DatabaseURL            string
	PostgresMaxConnections int32
	AllowedOrigin          string
	JWTSecret              string
	AccessTokenTTL         time.Duration
	RefreshTokenTTL        time.Duration
	CookieSecure           bool
	Argon2                 Argon2Config
	RateLimit              RateLimitConfig
	Redis                  RedisConfig
	RAGGRPCAddress         string
	RAGTimeout             time.Duration
}

// Load reads the configuration from the environment. It contains only the
// settings required by the current stage; new variables are added together
// with their consumers so that no configuration stays unused.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddress:            defaultHTTPAddr,
		LogLevel:               defaultLogLevel,
		PostgresMaxConnections: defaultPGMaxConns,
		AllowedOrigin:          defaultAllowedOrigin,
		AccessTokenTTL:         defaultAccessTokenTTL,
		RefreshTokenTTL:        defaultRefreshTokenTTL,
		Argon2: Argon2Config{
			Iterations:  defaultArgon2Iterations,
			MemoryMiB:   defaultArgon2MemoryMiB,
			Parallelism: defaultArgon2Parallelism,
		},
		RateLimit: RateLimitConfig{
			AuthPerMinute:    defaultRateLimitAuthPerMinute,
			RegisterPerHour:  defaultRateLimitRegisterPerHour,
			RefreshPerMinute: defaultRateLimitRefreshPerMinute,
		},
		Redis:          RedisConfig{Database: redisDefaultDB, Timeout: defaultRedisTimeout},
		RAGGRPCAddress: defaultRAGGRPCAddress,
		RAGTimeout:     defaultRAGTimeout,
	}

	if err := loadHTTP(&cfg); err != nil {
		return Config{}, err
	}
	if err := loadDatabase(&cfg); err != nil {
		return Config{}, err
	}
	if err := loadAuth(&cfg); err != nil {
		return Config{}, err
	}
	if err := loadArgon2(&cfg); err != nil {
		return Config{}, err
	}
	if err := loadRateLimit(&cfg); err != nil {
		return Config{}, err
	}
	if err := loadRedis(&cfg); err != nil {
		return Config{}, err
	}
	if err := loadRAG(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func loadHTTP(cfg *Config) error {
	if raw, ok := os.LookupEnv("HTTP_ADDR"); ok {
		cfg.HTTPAddress = strings.TrimSpace(raw)
	}
	if err := validateAddr(cfg.HTTPAddress); err != nil {
		return fmt.Errorf("HTTP_ADDR: %w", err)
	}

	level, err := LoadLogLevel()
	if err != nil {
		return err
	}
	cfg.LogLevel = level
	return nil
}

func loadDatabase(cfg *Config) error {
	databaseURL, err := LoadDatabaseURL()
	if err != nil {
		return err
	}
	cfg.DatabaseURL = databaseURL

	if raw, ok := os.LookupEnv("PG_MAX_CONNS"); ok {
		maxConns, err := parseInt("PG_MAX_CONNS", raw, 1, maxPGMaxConns)
		if err != nil {
			return err
		}
		//nolint:gosec // G115: maxConns is validated to fit into int32 above.
		cfg.PostgresMaxConnections = int32(maxConns)
	}
	return nil
}

// LoadDatabaseURL reads the database URL. The migration command uses it
// directly: applying migrations must not require API settings such as the JWT
// secret.
func LoadDatabaseURL() (string, error) {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return "", fmt.Errorf("DATABASE_URL: required")
	}
	return databaseURL, nil
}

// LoadLogLevel reads LOG_LEVEL with the info default.
func LoadLogLevel() (slog.Level, error) {
	raw, ok := os.LookupEnv("LOG_LEVEL")
	if !ok {
		return defaultLogLevel, nil
	}
	return parseLogLevel(raw)
}

func loadAuth(cfg *Config) error {
	if raw, ok := os.LookupEnv("ALLOWED_ORIGIN"); ok {
		cfg.AllowedOrigin = strings.TrimSpace(raw)
	}
	if err := validateOrigin(cfg.AllowedOrigin); err != nil {
		return fmt.Errorf("ALLOWED_ORIGIN: %w", err)
	}

	// The secret is required: an empty or short key would let anyone forge
	// access tokens.
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if len(secret) < minJWTSecretLength {
		return fmt.Errorf("JWT_SECRET: must be at least %d characters", minJWTSecretLength)
	}
	cfg.JWTSecret = secret

	accessTTL, err := secondsFromEnv("ACCESS_TOKEN_TTL", cfg.AccessTokenTTL, maxAccessTokenTTLSeconds)
	if err != nil {
		return err
	}
	cfg.AccessTokenTTL = accessTTL

	refreshTTL, err := secondsFromEnv("REFRESH_TOKEN_TTL", cfg.RefreshTokenTTL, maxRefreshTokenTTLSeconds)
	if err != nil {
		return err
	}
	cfg.RefreshTokenTTL = refreshTTL

	if raw, ok := os.LookupEnv("COOKIE_SECURE"); ok {
		secure, err := parseBool("COOKIE_SECURE", raw)
		if err != nil {
			return err
		}
		cfg.CookieSecure = secure
	}
	return nil
}

func loadArgon2(cfg *Config) error {
	if raw, ok := os.LookupEnv("ARGON2_ITERATIONS"); ok {
		value, err := parseInt("ARGON2_ITERATIONS", raw, 1, maxArgon2Iterations)
		if err != nil {
			return err
		}
		//nolint:gosec // G115: the value is validated to fit into uint32 above.
		cfg.Argon2.Iterations = uint32(value)
	}
	if raw, ok := os.LookupEnv("ARGON2_MEMORY_MIB"); ok {
		value, err := parseInt("ARGON2_MEMORY_MIB", raw, 8, maxArgon2MemoryMiB)
		if err != nil {
			return err
		}
		//nolint:gosec // G115: the value is validated to fit into uint32 above.
		cfg.Argon2.MemoryMiB = uint32(value)
	}
	if raw, ok := os.LookupEnv("ARGON2_PARALLELISM"); ok {
		value, err := parseInt("ARGON2_PARALLELISM", raw, 1, maxArgon2Parallelism)
		if err != nil {
			return err
		}
		//nolint:gosec // G115: the value is validated to fit into uint8 above.
		cfg.Argon2.Parallelism = uint8(value)
	}
	return nil
}

func loadRateLimit(cfg *Config) error {
	limits := []struct {
		key    string
		target *int64
	}{
		{"RATE_LIMIT_AUTH_PER_MINUTE", &cfg.RateLimit.AuthPerMinute},
		{"RATE_LIMIT_REGISTER_PER_HOUR", &cfg.RateLimit.RegisterPerHour},
		{"RATE_LIMIT_REFRESH_PER_MINUTE", &cfg.RateLimit.RefreshPerMinute},
	}
	for _, limit := range limits {
		raw, ok := os.LookupEnv(limit.key)
		if !ok {
			continue
		}
		value, err := parseInt(limit.key, raw, 1, maxRateLimitPerWindow)
		if err != nil {
			return err
		}
		*limit.target = int64(value)
	}
	return nil
}

func loadRedis(cfg *Config) error {
	cfg.Redis.Address = strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	cfg.Redis.Password = os.Getenv("REDIS_PASSWORD")

	if raw, ok := os.LookupEnv("REDIS_DB"); ok {
		value, err := parseInt("REDIS_DB", raw, 0, 15)
		if err != nil {
			return err
		}
		cfg.Redis.Database = value
	}
	timeout, err := secondsFromEnv("REDIS_TIMEOUT_SECONDS", cfg.Redis.Timeout, 60)
	if err != nil {
		return err
	}
	cfg.Redis.Timeout = timeout
	return nil
}

func loadRAG(cfg *Config) error {
	if raw, ok := os.LookupEnv("RAG_GRPC_ADDR"); ok {
		cfg.RAGGRPCAddress = strings.TrimSpace(raw)
	}
	if err := validateAddr(cfg.RAGGRPCAddress); err != nil {
		return fmt.Errorf("RAG_GRPC_ADDR: %w", err)
	}

	timeout, err := secondsFromEnv("RAG_TIMEOUT_SECONDS", cfg.RAGTimeout, maxRAGTimeoutSeconds)
	if err != nil {
		return err
	}
	cfg.RAGTimeout = timeout
	return nil
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

// validateOrigin requires an absolute http(s) origin: the value is compared
// with the Origin header of state-changing requests.
func validateOrigin(origin string) error {
	parsed, err := url.Parse(origin)
	if err != nil {
		return fmt.Errorf("expected absolute URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("expected http(s) origin, got %q", origin)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return fmt.Errorf("origin must not contain a path, got %q", origin)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("origin must not contain query or fragment, got %q", origin)
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

func parseInt(key, raw string, minValue, maxValue int) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < minValue || value > maxValue {
		return 0, fmt.Errorf("%s: expected integer in range %d..%d, got %q", key, minValue, maxValue, raw)
	}
	return value, nil
}

func secondsFromEnv(key string, fallback time.Duration, maxValue int) (time.Duration, error) {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	seconds, err := parseInt(key, raw, 1, maxValue)
	if err != nil {
		return 0, err
	}
	return time.Duration(seconds) * time.Second, nil
}

func parseBool(key, raw string) (bool, error) {
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("%s: expected boolean, got %q", key, raw)
	}
	return value, nil
}
