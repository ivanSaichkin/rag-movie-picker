//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"
	"time"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	ragv1 "github.com/ivanSaichkin/rag-movie-picker/pkg/ragapi/rag/v1"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/app"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/config"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/adapters/logger"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/persistence/postgres"
)

const (
	e2eOrigin = "http://frontend.example"
	e2eSecret = "e2e-secret-that-is-at-least-32-characters"
	e2eEmail  = "e2e@example.com"
)

func TestBackendEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	dsn := startPostgres(t)
	if err := postgres.Up(dsn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	ragAddress := startFakeRAG(t)
	httpAddress := freeAddress(t)

	cfg := config.Config{
		HTTPAddress:            httpAddress,
		LogLevel:               slog.LevelDebug,
		DatabaseURL:            dsn,
		PostgresMaxConnections: 5,
		AllowedOrigin:          e2eOrigin,
		JWTSecret:              e2eSecret,
		AccessTokenTTL:         15 * time.Minute,
		RefreshTokenTTL:        time.Hour,
		CookieSecure:           false,
		Argon2:                 config.Argon2Config{Iterations: 1, MemoryMiB: 8, Parallelism: 1},
		RateLimit:              config.RateLimitConfig{AuthPerMinute: 1000, RegisterPerHour: 1000, RefreshPerMinute: 1000},
		RAGGRPCAddress:         ragAddress,
		RAGTimeout:             2 * time.Second,
	}

	application, err := app.New(ctx, cfg, logger.New(slog.LevelDebug, io.Discard))
	if err != nil {
		t.Fatalf("build app: %v", err)
	}

	runErr := make(chan error, 1)
	go func() {
		runErr <- application.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-runErr:
			if err != nil {
				t.Errorf("app stopped with error: %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Error("app did not stop in time")
		}
	})

	baseURL := "http://" + httpAddress
	waitForHealth(t, baseURL+"/health")

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second}

	// Registration starts a session.
	statusCode, body := do(t, client, http.MethodPost, baseURL+"/api/v1/auth/register", map[string]string{
		"email":    e2eEmail,
		"password": "password123",
	}, "")
	requireStatus(t, statusCode, http.StatusCreated, body)

	var registered struct {
		AccessToken string `json:"access_token"`
		User        struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	unmarshal(t, body, &registered)
	if registered.AccessToken == "" || registered.User.Email != e2eEmail {
		t.Fatalf("registration response = %+v", registered)
	}
	accessToken := registered.AccessToken

	// The access token identifies the user.
	statusCode, body = do(t, client, http.MethodGet, baseURL+"/api/v1/auth/me", nil, accessToken)
	requireStatus(t, statusCode, http.StatusOK, body)
	var me struct {
		Email string `json:"email"`
	}
	unmarshal(t, body, &me)
	if me.Email != e2eEmail {
		t.Fatalf("me = %+v", me)
	}

	// The catalog comes from RAG through the adapter.
	statusCode, body = do(t, client, http.MethodGet, baseURL+"/api/v1/catalog", nil, accessToken)
	requireStatus(t, statusCode, http.StatusOK, body)
	var catalog struct {
		MovieCount      int64  `json:"movie_count"`
		DefaultStrategy string `json:"default_strategy"`
	}
	unmarshal(t, body, &catalog)
	if catalog.MovieCount != 4803 || catalog.DefaultStrategy != "hybrid" {
		t.Fatalf("catalog = %+v", catalog)
	}

	// A search is stored in the history.
	statusCode, body = do(t, client, http.MethodPost, baseURL+"/api/v1/recommendations", map[string]any{
		"query":   "космос",
		"limit":   5,
		"filters": map[string]any{"genre_ids": []int64{878}},
	}, accessToken)
	requireStatus(t, statusCode, http.StatusOK, body)

	var recommendation struct {
		HistoryID string `json:"history_id"`
		Mode      string `json:"mode"`
		Items     []struct {
			MovieID int64   `json:"movie_id"`
			Reason  *string `json:"reason"`
		} `json:"items"`
		Warnings []struct {
			Code string `json:"code"`
		} `json:"warnings"`
	}
	unmarshal(t, body, &recommendation)
	if recommendation.HistoryID == "" || recommendation.Mode != "generated" {
		t.Fatalf("recommendation = %+v", recommendation)
	}
	if len(recommendation.Items) != 1 || recommendation.Items[0].MovieID != 603 {
		t.Fatalf("items = %+v", recommendation.Items)
	}
	if recommendation.Items[0].Reason == nil || len(recommendation.Warnings) == 0 {
		t.Fatalf("reason and warnings must be present: %+v", recommendation)
	}

	statusCode, body = do(t, client, http.MethodGet, baseURL+"/api/v1/history", nil, accessToken)
	requireStatus(t, statusCode, http.StatusOK, body)
	var historyList struct {
		Total int64 `json:"total"`
		Items []struct {
			HistoryID string `json:"history_id"`
		} `json:"items"`
	}
	unmarshal(t, body, &historyList)
	if historyList.Total != 1 || historyList.Items[0].HistoryID != recommendation.HistoryID {
		t.Fatalf("history list = %+v", historyList)
	}

	statusCode, body = do(t, client, http.MethodGet, baseURL+"/api/v1/history/"+recommendation.HistoryID, nil, accessToken)
	requireStatus(t, statusCode, http.StatusOK, body)
	var historyEntry struct {
		Query    string `json:"query"`
		Response struct {
			Mode  string `json:"mode"`
			Items []struct {
				MovieID int64 `json:"movie_id"`
			} `json:"items"`
		} `json:"response"`
	}
	unmarshal(t, body, &historyEntry)
	if historyEntry.Query != "космос" || historyEntry.Response.Mode != "generated" || len(historyEntry.Response.Items) != 1 {
		t.Fatalf("history entry = %+v", historyEntry)
	}

	// Personal lists: first write takes the card from RAG.
	statusCode, body = do(t, client, http.MethodPut, baseURL+"/api/v1/my/movies/603", map[string]any{
		"status": "want_to_watch",
	}, accessToken)
	requireStatus(t, statusCode, http.StatusOK, body)
	var listed struct {
		MovieID int64  `json:"movie_id"`
		Status  string `json:"status"`
		Liked   *bool  `json:"liked"`
	}
	unmarshal(t, body, &listed)
	if listed.Status != "want_to_watch" || listed.Liked != nil {
		t.Fatalf("listed movie = %+v", listed)
	}

	statusCode, body = do(t, client, http.MethodGet, baseURL+"/api/v1/my/movies?status=want_to_watch", nil, accessToken)
	requireStatus(t, statusCode, http.StatusOK, body)
	var wantList struct {
		Total int64 `json:"total"`
		Items []struct {
			MovieID int64 `json:"movie_id"`
		} `json:"items"`
	}
	unmarshal(t, body, &wantList)
	if wantList.Total != 1 || wantList.Items[0].MovieID != 603 {
		t.Fatalf("want list = %+v", wantList)
	}

	statusCode, body = do(t, client, http.MethodPut, baseURL+"/api/v1/my/movies/603", map[string]any{
		"status": "watched",
		"liked":  true,
	}, accessToken)
	requireStatus(t, statusCode, http.StatusOK, body)
	unmarshal(t, body, &listed)
	if listed.Status != "watched" || listed.Liked == nil || !*listed.Liked {
		t.Fatalf("watched movie = %+v", listed)
	}

	statusCode, body = do(t, client, http.MethodPut, baseURL+"/api/v1/my/movies/603", map[string]any{
		"status": "want_to_watch",
	}, accessToken)
	requireStatus(t, statusCode, http.StatusOK, body)
	unmarshal(t, body, &listed)
	if listed.Liked != nil {
		t.Fatalf("moving back must clear the rating: %+v", listed)
	}

	// Refresh rotation: the old cookie must be rejected, the current one works.
	authURL, err := url.Parse(baseURL + "/api/v1/auth")
	if err != nil {
		t.Fatalf("parse auth url: %v", err)
	}
	oldRefresh := refreshCookieValue(t, jar, authURL)

	statusCode, body = do(t, client, http.MethodPost, baseURL+"/api/v1/auth/refresh", nil, "")
	requireStatus(t, statusCode, http.StatusOK, body)
	var refreshed struct {
		AccessToken string `json:"access_token"`
	}
	unmarshal(t, body, &refreshed)
	if refreshed.AccessToken == "" {
		t.Fatal("refresh did not return an access token")
	}

	reuseRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/auth/refresh", nil)
	if err != nil {
		t.Fatalf("build reuse request: %v", err)
	}
	reuseRequest.Header.Set("Cookie", "refresh_token="+oldRefresh)
	reuseRequest.Header.Set("Origin", e2eOrigin)
	reuseResponse, err := (&http.Client{Timeout: 15 * time.Second}).Do(reuseRequest)
	if err != nil {
		t.Fatalf("reuse refresh: %v", err)
	}
	if err := reuseResponse.Body.Close(); err != nil {
		t.Fatalf("close reuse response: %v", err)
	}
	if reuseResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("reused refresh status = %d, want 401", reuseResponse.StatusCode)
	}

	statusCode, body = do(t, client, http.MethodPost, baseURL+"/api/v1/auth/logout", nil, refreshed.AccessToken)
	requireStatus(t, statusCode, http.StatusNoContent, body)

	statusCode, body = do(t, client, http.MethodPost, baseURL+"/api/v1/auth/refresh", nil, "")
	requireStatus(t, statusCode, http.StatusUnauthorized, body)

	// Validation still reports the contract error.
	statusCode, body = do(t, client, http.MethodPost, baseURL+"/api/v1/recommendations", map[string]any{
		"query": "   ",
	}, accessToken)
	requireStatus(t, statusCode, http.StatusUnprocessableEntity, body)

	// Deleting history removes the stored snapshot.
	statusCode, body = do(t, client, http.MethodDelete, baseURL+"/api/v1/history/"+recommendation.HistoryID, nil, accessToken)
	requireStatus(t, statusCode, http.StatusNoContent, body)

	statusCode, body = do(t, client, http.MethodGet, baseURL+"/api/v1/history/"+recommendation.HistoryID, nil, accessToken)
	requireStatus(t, statusCode, http.StatusNotFound, body)
}

func startPostgres(t *testing.T) string {
	t.Helper()

	ctx := context.Background()
	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("app"),
		tcpostgres.WithUsername("app"),
		tcpostgres.WithPassword("app"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("terminate postgres: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("read connection string: %v", err)
	}
	return dsn
}

type fakeRAGServer struct {
	ragv1.UnimplementedRagServiceServer
}

func startFakeRAG(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for fake RAG: %v", err)
	}

	server := grpc.NewServer()
	ragv1.RegisterRagServiceServer(server, fakeRAGServer{})
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	return listener.Addr().String()
}

func (fakeRAGServer) GetCatalog(context.Context, *ragv1.GetCatalogRequest) (*ragv1.Catalog, error) {
	return &ragv1.Catalog{
		Genres:              []*ragv1.Genre{{Id: 878, Name: "Science Fiction"}},
		MovieCount:          4803,
		YearFrom:            proto.Int32(1902),
		YearTo:              proto.Int32(2017),
		DefaultResults:      10,
		MaxResults:          50,
		AvailableStrategies: []string{"bm25", "transformer", "hybrid"},
		DefaultStrategy:     "hybrid",
	}, nil
}

func (fakeRAGServer) GetMovie(_ context.Context, request *ragv1.GetMovieRequest) (*ragv1.Movie, error) {
	if request.GetMovieId() == 404 {
		return nil, status.Error(codes.NotFound, "movie not found")
	}
	return &ragv1.Movie{
		MovieId:   request.GetMovieId(),
		Title:     "The Matrix",
		Year:      proto.Int32(1999),
		Genres:    []*ragv1.Genre{{Id: 28, Name: "Action"}},
		Rating:    proto.Float64(8.2),
		VoteCount: 24589,
		Overview:  "A hacker discovers the truth.",
	}, nil
}

func (fakeRAGServer) Recommend(context.Context, *ragv1.RecommendRequest) (*ragv1.RecommendResponse, error) {
	return &ragv1.RecommendResponse{
		Mode:         "generated",
		StrategyUsed: "hybrid",
		Items: []*ragv1.Recommendation{{
			Movie:  &ragv1.Movie{MovieId: 603, Title: "The Matrix"},
			Reason: proto.String("подходит под запрос"),
		}},
		Warnings: []*ragv1.Warning{{Code: "FEWER_RESULTS", Message: "меньше, чем просили"}},
	}, nil
}

func do(t *testing.T, client *http.Client, method, endpoint string, payload any, accessToken string) (int, []byte) {
	t.Helper()

	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("encode request body: %v", err)
		}
		body = bytes.NewReader(raw)
	}

	request, err := http.NewRequestWithContext(context.Background(), method, endpoint, body)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Origin", e2eOrigin)
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if accessToken != "" {
		request.Header.Set("Authorization", "Bearer "+accessToken)
	}

	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, endpoint, err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Errorf("close response body: %v", err)
		}
	}()

	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return response.StatusCode, data
}

func requireStatus(t *testing.T, got, want int, body []byte) {
	t.Helper()
	if got != want {
		t.Fatalf("status = %d, want %d, body = %s", got, want, body)
	}
}

func unmarshal(t *testing.T, body []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("decode response: %v, body = %s", err, body)
	}
}

func refreshCookieValue(t *testing.T, jar http.CookieJar, base *url.URL) string {
	t.Helper()

	for _, cookie := range jar.Cookies(base) {
		if cookie.Name == "refresh_token" {
			return cookie.Value
		}
	}
	t.Fatal("refresh cookie is missing")
	return ""
}

func freeAddress(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release address: %v", err)
	}
	return address
}

func waitForHealth(t *testing.T, endpoint string) {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	for {
		response, err := http.Get(endpoint)
		if err == nil {
			if err := response.Body.Close(); err != nil {
				t.Fatalf("close health response: %v", err)
			}
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("api did not become healthy: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
