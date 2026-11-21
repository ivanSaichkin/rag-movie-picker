package history_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/infrastructure/services/history"
)

var testUserID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

type fakeRepository struct {
	items   []models.HistoryListItem
	total   int64
	listErr error
	limit   int
	offset  int

	entry     models.HistoryEntry
	getErr    error
	deleted   bool
	deleteErr error
}

func (f *fakeRepository) Create(context.Context, models.HistoryEntry) (models.HistoryEntry, error) {
	return models.HistoryEntry{}, errors.New("unexpected call")
}

func (f *fakeRepository) List(_ context.Context, _ uuid.UUID, limit, offset int) ([]models.HistoryListItem, int64, error) {
	f.limit = limit
	f.offset = offset
	return f.items, f.total, f.listErr
}

func (f *fakeRepository) Get(context.Context, uuid.UUID, uuid.UUID) (models.HistoryEntry, error) {
	return f.entry, f.getErr
}

func (f *fakeRepository) Delete(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return f.deleted, f.deleteErr
}

func TestListCalculatesOffset(t *testing.T) {
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	repository := &fakeRepository{
		items: []models.HistoryListItem{{ID: uuid.New(), Query: "космос", RequestedLimit: 10, CreatedAt: createdAt}},
		total: 42,
	}
	service := history.NewService(repository)

	page, err := service.List(context.Background(), testUserID, 3, 20)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if repository.limit != 20 || repository.offset != 40 {
		t.Fatalf("repository received limit=%d offset=%d", repository.limit, repository.offset)
	}
	if page.Page != 3 || page.PageSize != 20 || page.Total != 42 {
		t.Fatalf("page = %+v", page)
	}
	if len(page.Items) != 1 || page.Items[0].Query != "космос" {
		t.Fatalf("items = %+v", page.Items)
	}
}

func TestListPropagatesFailure(t *testing.T) {
	repository := &fakeRepository{listErr: errors.New("database is down")}
	service := history.NewService(repository)

	if _, err := service.List(context.Background(), testUserID, 1, 20); err == nil {
		t.Fatal("List() must propagate the repository error")
	}
}

func TestGetReturnsStoredSnapshot(t *testing.T) {
	entry := models.HistoryEntry{ID: uuid.New(), Query: "космос", Response: models.Recommendation{Mode: "generated"}}
	service := history.NewService(&fakeRepository{entry: entry})

	found, err := service.Get(context.Background(), testUserID, entry.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if found.ID != entry.ID || found.Query != "космос" || found.Response.Mode != "generated" {
		t.Fatalf("entry = %+v", found)
	}
}

func TestDeleteReportsMissingRecord(t *testing.T) {
	service := history.NewService(&fakeRepository{deleted: false})

	if err := service.Delete(context.Background(), testUserID, uuid.New()); !errors.Is(err, rules.ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestDeleteRemovesRecord(t *testing.T) {
	service := history.NewService(&fakeRepository{deleted: true})

	if err := service.Delete(context.Background(), testUserID, uuid.New()); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestDeletePropagatesFailure(t *testing.T) {
	service := history.NewService(&fakeRepository{deleteErr: errors.New("database is down")})

	if err := service.Delete(context.Background(), testUserID, uuid.New()); err == nil {
		t.Fatal("Delete() must propagate the repository error")
	}
}
