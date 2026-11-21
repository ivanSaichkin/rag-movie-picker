package app

import (
	"fmt"
	"log/slog"

	ragv1 "github.com/ivanSaichkin/rag-movie-picker/pkg/ragapi/rag/v1"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/config"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/password"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/token"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/clients/ragclient"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/persistence/postgres"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/services/auth"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/services/catalog"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/services/cleanup"
)

// services is the application core: the readiness check, the business services
// and the background cleaner.
type services struct {
	readinessCheck        ports.ReadinessCheck
	authenticationService ports.AuthenticationService
	catalogService        ports.CatalogService
	tokenIssuer           ports.AccessTokenIssuer
	cleaner               *cleanup.RefreshTokenCleaner
}

// newServices builds the repositories and services over the opened resources.
func newServices(infra *infrastructure, cfg config.Config, logger *slog.Logger) (*services, error) {
	readinessCheck, err := postgres.NewReadinessCheck(infra.pool)
	if err != nil {
		return nil, fmt.Errorf("build readiness check: %w", err)
	}

	users := postgres.NewUserRepository(infra.pool)
	refreshTokens := postgres.NewRefreshTokenRepository(infra.pool)

	hasher := password.NewArgon2idHasher(cfg.Argon2.Iterations, cfg.Argon2.MemoryMiB, cfg.Argon2.Parallelism)
	tokenIssuer := token.NewAccessTokenIssuer(cfg.JWTSecret, cfg.AccessTokenTTL)

	authenticationService, err := auth.NewAuthenticationService(
		users,
		refreshTokens,
		hasher,
		tokenIssuer,
		cfg.RefreshTokenTTL,
	)
	if err != nil {
		return nil, fmt.Errorf("build auth service: %w", err)
	}

	ragClient := ragclient.NewRAGClient(ragv1.NewRagServiceClient(infra.ragConnection), cfg.RAGTimeout)

	return &services{
		readinessCheck:        readinessCheck,
		authenticationService: authenticationService,
		catalogService:        catalog.NewService(ragClient, infra.cache, logger),
		tokenIssuer:           tokenIssuer,
		cleaner:               cleanup.NewRefreshTokenCleaner(refreshTokens, logger),
	}, nil
}
