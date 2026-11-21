package ragclient_test

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	ragv1 "github.com/ivanSaichkin/rag-movie-picker/pkg/ragapi/rag/v1"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/clients/ragclient"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/platform/requestid"
)

const testTimeout = time.Second

type fakeRAGServer struct {
	ragv1.UnimplementedRagServiceServer

	mu               sync.Mutex
	catalog          *ragv1.Catalog
	movie            *ragv1.Movie
	recommend        *ragv1.RecommendResponse
	err              error
	delay            time.Duration
	requestID        string
	movieID          int64
	recommendRequest *ragv1.RecommendRequest
	calls            int
}

func (s *fakeRAGServer) GetCatalog(ctx context.Context, _ *ragv1.GetCatalogRequest) (*ragv1.Catalog, error) {
	s.record(ctx)
	if err := s.wait(ctx); err != nil {
		return nil, err
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.catalog, nil
}

func (s *fakeRAGServer) GetMovie(ctx context.Context, request *ragv1.GetMovieRequest) (*ragv1.Movie, error) {
	s.record(ctx)
	s.mu.Lock()
	s.movieID = request.GetMovieId()
	s.mu.Unlock()
	if err := s.wait(ctx); err != nil {
		return nil, err
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.movie, nil
}

func (s *fakeRAGServer) Recommend(ctx context.Context, request *ragv1.RecommendRequest) (*ragv1.RecommendResponse, error) {
	s.record(ctx)
	s.mu.Lock()
	s.recommendRequest = request
	s.mu.Unlock()
	if err := s.wait(ctx); err != nil {
		return nil, err
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.recommend, nil
}

func (s *fakeRAGServer) capturedRecommend() *ragv1.RecommendRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recommendRequest
}

func (s *fakeRAGServer) record(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.calls++
	if incoming, ok := metadata.FromIncomingContext(ctx); ok {
		if values := incoming.Get("x-request-id"); len(values) > 0 {
			s.requestID = values[0]
		}
	}
}

func (s *fakeRAGServer) wait(ctx context.Context) error {
	if s.delay <= 0 {
		return nil
	}
	select {
	case <-time.After(s.delay):
		return nil
	case <-ctx.Done():
		return status.FromContextError(ctx.Err()).Err()
	}
}

func (s *fakeRAGServer) captured() (requestID string, movieID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requestID, s.movieID
}

func newTestClient(t *testing.T, server *fakeRAGServer, timeout time.Duration) *ragclient.RAGClient {
	t.Helper()

	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	ragv1.RegisterRagServiceServer(grpcServer, server)
	go func() {
		_ = grpcServer.Serve(listener)
	}()
	t.Cleanup(grpcServer.Stop)

	connection, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return listener.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		_ = connection.Close()
	})

	return ragclient.NewRAGClient(ragv1.NewRagServiceClient(connection), timeout)
}

func sampleCatalog() *ragv1.Catalog {
	return &ragv1.Catalog{
		Genres: []*ragv1.Genre{
			{Id: 878, Name: "Science Fiction"},
			{Id: 12, Name: "Adventure"},
		},
		MovieCount:          4803,
		YearFrom:            proto.Int32(1902),
		YearTo:              proto.Int32(2017),
		DefaultResults:      10,
		MaxResults:          50,
		AvailableStrategies: []string{"bm25", "transformer", "hybrid"},
		DefaultStrategy:     "hybrid",
	}
}

func TestCatalogMapsFieldsAndRequestID(t *testing.T) {
	server := &fakeRAGServer{catalog: sampleCatalog()}
	client := newTestClient(t, server, testTimeout)

	ctx := requestid.WithID(context.Background(), "req-42")
	catalog, err := client.Catalog(ctx)
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}

	if len(catalog.Genres) != 2 || catalog.Genres[0].ID != 878 || catalog.Genres[0].Name != "Science Fiction" {
		t.Fatalf("genres = %+v", catalog.Genres)
	}
	if catalog.MovieCount != 4803 {
		t.Fatalf("MovieCount = %d", catalog.MovieCount)
	}
	if catalog.YearFrom == nil || *catalog.YearFrom != 1902 || catalog.YearTo == nil || *catalog.YearTo != 2017 {
		t.Fatalf("years = %v, %v", catalog.YearFrom, catalog.YearTo)
	}
	if catalog.DefaultResults != 10 || catalog.MaxResults != 50 {
		t.Fatalf("limits = %d, %d", catalog.DefaultResults, catalog.MaxResults)
	}
	if len(catalog.AvailableStrategies) != 3 || catalog.DefaultStrategy != "hybrid" {
		t.Fatalf("strategies = %v, %q", catalog.AvailableStrategies, catalog.DefaultStrategy)
	}

	if requestID, _ := server.captured(); requestID != "req-42" {
		t.Fatalf("request id metadata = %q, want req-42", requestID)
	}
}

func TestCatalogKeepsUnknownBounds(t *testing.T) {
	server := &fakeRAGServer{catalog: &ragv1.Catalog{MovieCount: 10}}
	client := newTestClient(t, server, testTimeout)

	catalog, err := client.Catalog(context.Background())
	if err != nil {
		t.Fatalf("Catalog() error = %v", err)
	}
	if catalog.YearFrom != nil || catalog.YearTo != nil {
		t.Fatalf("years = %v, %v, want nil", catalog.YearFrom, catalog.YearTo)
	}
}

func TestMovieMapsFields(t *testing.T) {
	server := &fakeRAGServer{movie: &ragv1.Movie{
		MovieId:   603,
		Title:     "The Matrix",
		Year:      proto.Int32(1999),
		Genres:    []*ragv1.Genre{{Id: 28, Name: "Action"}},
		Rating:    proto.Float64(8.2),
		VoteCount: 24589,
		Overview:  "A hacker discovers the truth.",
	}}
	client := newTestClient(t, server, testTimeout)

	movie, err := client.Movie(context.Background(), 603)
	if err != nil {
		t.Fatalf("Movie() error = %v", err)
	}
	if movie.MovieID != 603 || movie.Title != "The Matrix" {
		t.Fatalf("movie = %+v", movie)
	}
	if movie.Year == nil || *movie.Year != 1999 || movie.Rating == nil || *movie.Rating != 8.2 {
		t.Fatalf("optional fields = %v, %v", movie.Year, movie.Rating)
	}
	if movie.VoteCount != 24589 || movie.Overview == "" {
		t.Fatalf("movie = %+v", movie)
	}

	if _, movieID := server.captured(); movieID != 603 {
		t.Fatalf("requested movie id = %d", movieID)
	}
}

func TestMovieKeepsUnknownOptionalFields(t *testing.T) {
	server := &fakeRAGServer{movie: &ragv1.Movie{MovieId: 1, Title: "Untitled"}}
	client := newTestClient(t, server, testTimeout)

	movie, err := client.Movie(context.Background(), 1)
	if err != nil {
		t.Fatalf("Movie() error = %v", err)
	}
	if movie.Year != nil || movie.Rating != nil {
		t.Fatalf("optional fields = %v, %v, want nil", movie.Year, movie.Rating)
	}
}

func TestMapsGRPCStatuses(t *testing.T) {
	cases := map[codes.Code]error{
		codes.InvalidArgument:  rules.ErrInvalidArgument,
		codes.NotFound:         rules.ErrNotFound,
		codes.Unavailable:      rules.ErrUnavailable,
		codes.DeadlineExceeded: rules.ErrDeadlineExceeded,
	}

	for code, want := range cases {
		t.Run(code.String(), func(t *testing.T) {
			server := &fakeRAGServer{err: status.Error(code, "boom")}
			client := newTestClient(t, server, testTimeout)

			_, err := client.Catalog(context.Background())
			if !errors.Is(err, want) {
				t.Fatalf("Catalog() error = %v, want %v", err, want)
			}
		})
	}
}

func TestUnexpectedStatusCodeKeepsCause(t *testing.T) {
	server := &fakeRAGServer{err: status.Error(codes.Internal, "index is broken")}
	client := newTestClient(t, server, testTimeout)

	_, err := client.Catalog(context.Background())
	if err == nil {
		t.Fatal("Catalog() must fail")
	}
	if errors.Is(err, rules.ErrUnavailable) || errors.Is(err, rules.ErrDeadlineExceeded) {
		t.Fatalf("internal error must not look like an outage: %v", err)
	}
	if !strings.Contains(err.Error(), "index is broken") {
		t.Fatalf("error %q must keep the cause", err)
	}
}

func TestDeadlineIsAppliedToTheCall(t *testing.T) {
	server := &fakeRAGServer{catalog: sampleCatalog(), delay: 300 * time.Millisecond}
	client := newTestClient(t, server, 30*time.Millisecond)

	_, err := client.Catalog(context.Background())
	if !errors.Is(err, rules.ErrDeadlineExceeded) {
		t.Fatalf("Catalog() error = %v, want ErrDeadlineExceeded", err)
	}
}

func TestRecommendMapsRequestAndResponse(t *testing.T) {
	server := &fakeRAGServer{recommend: &ragv1.RecommendResponse{
		Mode:         "generated",
		StrategyUsed: "hybrid",
		Items: []*ragv1.Recommendation{{
			Movie:  &ragv1.Movie{MovieId: 603, Title: "The Matrix"},
			Reason: proto.String("подходит под запрос"),
		}},
		Warnings: []*ragv1.Warning{{Code: "FEWER_RESULTS", Message: "меньше, чем просили"}},
	}}
	client := newTestClient(t, server, testTimeout)

	limit := 5
	yearFrom, yearTo := int32(2000), int32(2015)
	rating := 6.5
	strategy := "hybrid"
	generate := false
	response, err := client.Recommend(context.Background(), models.RecommendationRequest{
		Query: "космос",
		Limit: &limit,
		Filters: models.Filters{
			GenreIDs:  []int64{878},
			YearFrom:  &yearFrom,
			YearTo:    &yearTo,
			MinRating: &rating,
		},
		Strategy:             &strategy,
		GenerateExplanations: &generate,
	})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}

	if response.Mode != "generated" || response.StrategyUsed != "hybrid" {
		t.Fatalf("response = %+v", response)
	}
	if len(response.Items) != 1 || response.Items[0].Movie.MovieID != 603 {
		t.Fatalf("items = %+v", response.Items)
	}
	if response.Items[0].Reason == nil || *response.Items[0].Reason != "подходит под запрос" {
		t.Fatalf("reason = %v", response.Items[0].Reason)
	}
	if len(response.Warnings) != 1 || response.Warnings[0].Code != "FEWER_RESULTS" {
		t.Fatalf("warnings = %+v", response.Warnings)
	}

	request := server.capturedRecommend()
	if request == nil {
		t.Fatal("server did not receive the request")
	}
	if request.GetQuery() != "космос" || request.GetLimit() != 5 {
		t.Fatalf("request = query %q, limit %d", request.GetQuery(), request.GetLimit())
	}
	if request.GetFilters().GetGenreIds()[0] != 878 {
		t.Fatalf("filters = %+v", request.GetFilters())
	}
	if request.GetFilters().YearFrom == nil || *request.GetFilters().YearFrom != 2000 {
		t.Fatalf("year_from = %v", request.GetFilters().YearFrom)
	}
	if request.GetFilters().MinRating == nil || *request.GetFilters().MinRating != 6.5 {
		t.Fatalf("min_rating = %v", request.GetFilters().MinRating)
	}
	if request.Strategy == nil || *request.Strategy != "hybrid" {
		t.Fatalf("strategy = %v", request.Strategy)
	}
	if request.GenerateExplanations == nil || *request.GenerateExplanations {
		t.Fatalf("generate_explanations = %v", request.GenerateExplanations)
	}
}

func TestRecommendKeepsAbsentOptionalFields(t *testing.T) {
	server := &fakeRAGServer{recommend: &ragv1.RecommendResponse{Mode: "empty"}}
	client := newTestClient(t, server, testTimeout)

	response, err := client.Recommend(context.Background(), models.RecommendationRequest{Query: "космос"})
	if err != nil {
		t.Fatalf("Recommend() error = %v", err)
	}
	if len(response.Items) != 0 || len(response.Warnings) != 0 {
		t.Fatalf("response = %+v", response)
	}

	request := server.capturedRecommend()
	if request.Limit != nil || request.Strategy != nil || request.GenerateExplanations != nil {
		t.Fatalf("absent fields must stay absent: %+v", request)
	}
	if request.GetFilters() == nil {
		t.Fatal("filters must always be sent")
	}
}
