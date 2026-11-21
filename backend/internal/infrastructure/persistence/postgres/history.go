package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
)

// History stores recommendation snapshots in PostgreSQL.
type History struct {
	pool *pgxpool.Pool
}

// NewHistoryRepository builds the history repository.
func NewHistoryRepository(pool *pgxpool.Pool) *History {
	return &History{pool: pool}
}

// Create inserts a snapshot and returns the generated identifier and creation
// time.
func (r *History) Create(ctx context.Context, entry models.HistoryEntry) (models.HistoryEntry, error) {
	filters, err := encodeStoredFilters(entry.Filters)
	if err != nil {
		return models.HistoryEntry{}, err
	}
	response, err := encodeStoredResponse(entry.Response)
	if err != nil {
		return models.HistoryEntry{}, err
	}

	const query = `
		INSERT INTO search_history (user_id, query, filters, requested_limit, response)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`

	err = r.pool.QueryRow(ctx, query, entry.UserID, entry.Query, filters, entry.RequestedLimit, response).
		Scan(&entry.ID, &entry.CreatedAt)
	if err != nil {
		return models.HistoryEntry{}, fmt.Errorf("create history entry: %w", err)
	}
	return entry, nil
}

// List returns one page of the user's history and the total count.
func (r *History) List(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.HistoryListItem, int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM search_history WHERE user_id = $1`, userID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count history entries: %w", err)
	}

	const query = `
		SELECT id, query, filters, requested_limit, created_at
		FROM search_history
		WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list history entries: %w", err)
	}
	defer rows.Close()

	items := make([]models.HistoryListItem, 0, limit)
	for rows.Next() {
		var (
			item       models.HistoryListItem
			rawFilters []byte
		)
		if err := rows.Scan(&item.ID, &item.Query, &rawFilters, &item.RequestedLimit, &item.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan history entry: %w", err)
		}
		filters, err := decodeStoredFilters(rawFilters)
		if err != nil {
			return nil, 0, err
		}
		item.Filters = filters
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate history entries: %w", err)
	}
	return items, total, nil
}

// Get returns one snapshot of the user.
func (r *History) Get(ctx context.Context, userID, historyID uuid.UUID) (models.HistoryEntry, error) {
	const query = `
		SELECT id, user_id, query, filters, requested_limit, response, created_at
		FROM search_history
		WHERE id = $1 AND user_id = $2`

	var (
		entry       models.HistoryEntry
		rawFilters  []byte
		rawResponse []byte
	)
	err := r.pool.QueryRow(ctx, query, historyID, userID).Scan(
		&entry.ID,
		&entry.UserID,
		&entry.Query,
		&rawFilters,
		&entry.RequestedLimit,
		&rawResponse,
		&entry.CreatedAt,
	)
	if err != nil {
		return models.HistoryEntry{}, wrapQueryError("find history entry", err)
	}

	filters, err := decodeStoredFilters(rawFilters)
	if err != nil {
		return models.HistoryEntry{}, err
	}
	response, err := decodeStoredResponse(rawResponse)
	if err != nil {
		return models.HistoryEntry{}, err
	}
	entry.Filters = filters
	entry.Response = response
	return entry, nil
}

// Delete removes the user's snapshot. It reports whether a row was deleted.
func (r *History) Delete(ctx context.Context, userID, historyID uuid.UUID) (bool, error) {
	const query = `DELETE FROM search_history WHERE id = $1 AND user_id = $2`

	tag, err := r.pool.Exec(ctx, query, historyID, userID)
	if err != nil {
		return false, fmt.Errorf("delete history entry: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

var _ ports.HistoryRepository = (*History)(nil)
