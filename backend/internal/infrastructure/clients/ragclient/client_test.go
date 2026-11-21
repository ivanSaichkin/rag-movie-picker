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

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/clients/ragclient"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/platform/requestid"
)

const testTimeout = time.Second

type fakeRAGServer struct {
	ragv1.UnimplementedRagServiceServer

	mu        sync.Mutex
	catalog   *ragv1.Catalog
	movie     *ragv1.Movie
	err       error
	delay     time.Duration
	requestID string
	movieID   int64
	calls     int
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
