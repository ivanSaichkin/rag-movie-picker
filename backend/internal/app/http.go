package app

import (
	"log/slog"
	"time"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/config"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/router"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/handlers"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
)

// newHTTPServer builds the handlers, the middleware chain and the HTTP server.
func newHTTPServer(cfg config.Config, logger *slog.Logger, infra *infrastructure, appServices *services) *server.Server {
	authHandler := handlers.NewAuthHandler(appServices.authenticationService, cfg.CookieSecure)
	catalogHandler := handlers.NewCatalogHandler(appServices.catalogService)
	recommendationHandler := handlers.NewRecommendationHandler(appServices.recommendationService)
	historyHandler := handlers.NewHistoryHandler(appServices.historyService)
	userMovieHandler := handlers.NewUserMovieHandler(appServices.userMovieService)
	healthHandler := handlers.NewHealthHandler(appServices.readinessCheck)

	engine := router.New(router.Params{
		Logger:         logger,
		Health:         healthHandler,
		Auth:           authHandler,
		Catalog:        catalogHandler,
		Recommendation: recommendationHandler,
		History:        historyHandler,
		UserMovies:     userMovieHandler,
		Authenticate:   middleware.Authenticate(appServices.tokenIssuer),
		AllowedOrigin:  cfg.AllowedOrigin,
		Limiters: router.Limiters{
			Login:    middleware.RateLimit(infra.cache, "login", cfg.RateLimit.AuthPerMinute, time.Minute),
			Register: middleware.RateLimit(infra.cache, "register", cfg.RateLimit.RegisterPerHour, time.Hour),
			Refresh:  middleware.RateLimit(infra.cache, "refresh", cfg.RateLimit.RefreshPerMinute, time.Minute),
		},
	})

	return server.New(cfg.HTTPAddress, engine, logger)
}
