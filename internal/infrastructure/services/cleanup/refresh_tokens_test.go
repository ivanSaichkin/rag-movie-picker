package cleanup

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/internal/domain/models"
)

type fakeRepository struct {
	mu      sync.Mutex
	calls   int
	cutoffs []time.Time
	removed int64
	err     error
}

func (f *fakeRepository) Create(context.Context, models.RefreshToken) (models.RefreshToken, error) {
	return models.RefreshToken{}, errors.New("unexpected call")
}

func (f *fakeRepository) ByHash(context.Context, []byte) (models.RefreshToken, error) {
	return models.RefreshToken{}, errors.New("unexpected call")
}

func (f *fakeRepository) Revoke(context.Context, uuid.UUID) (bool, error) {
	return false, errors.New("unexpected call")
}

func (f *fakeRepository) RevokeFamily(context.Context, uuid.UUID) (int64, error) {
	return 0, errors.New("unexpected call")
}

func (f *fakeRepository) DeleteStale(_ context.Context, cutoff time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++
	f.cutoffs = append(f.cutoffs, cutoff)
	return f.removed, f.err
}

func (f *fakeRepository) snapshot() (int, []time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.calls, append([]time.Time(nil), f.cutoffs...)
}

func TestRunCleansUpOnStartAndStopsOnCancel(t *testing.T) {
	repository := &fakeRepository{removed: 3}
	var buffer bytes.Buffer

	task := NewRefreshTokens(repository, slog.New(slog.NewJSONHandler(&buffer, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Run performs the initial cleanup even when the context is already done.
	task.Run(ctx)

	calls, cutoffs := repository.snapshot()
	if calls != 1 {
		t.Fatalf("DeleteStale calls = %d, want 1", calls)
	}

	want := time.Now().Add(-defaultRetention)
	if len(cutoffs) != 1 || cutoffs[0].Sub(want).Abs() > time.Minute {
		t.Fatalf("cutoff = %v, want about %v", cutoffs, want)
	}
	if !strings.Contains(buffer.String(), "stale refresh tokens removed") {
		t.Fatalf("removal must be logged, logs = %s", buffer.String())
	}
}

func TestRunLogsCleanupFailure(t *testing.T) {
	repository := &fakeRepository{err: errors.New("database is down")}
	var buffer bytes.Buffer

	task := NewRefreshTokens(repository, slog.New(slog.NewJSONHandler(&buffer, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	task.Run(ctx)

	if !strings.Contains(buffer.String(), "refresh token cleanup failed") {
		t.Fatalf("failure must be logged, logs = %s", buffer.String())
	}
	if !strings.Contains(buffer.String(), "database is down") {
		t.Fatalf("cause must be logged, logs = %s", buffer.String())
	}
}

func TestRunDoesNotLogWhenNothingRemoved(t *testing.T) {
	repository := &fakeRepository{}
	var buffer bytes.Buffer

	task := NewRefreshTokens(repository, slog.New(slog.NewJSONHandler(&buffer, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	task.Run(ctx)

	if strings.Contains(buffer.String(), "removed") {
		t.Fatalf("quiet cleanup must not log a removal, logs = %s", buffer.String())
	}
}
