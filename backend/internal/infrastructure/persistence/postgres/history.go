package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

	// The chat and first snapshot commit together. A lock on an existing
	// chat serializes concurrent appends and deletion.
	chatQuery := `INSERT INTO chats (user_id) VALUES ($1) RETURNING id`
	args := []any{entry.UserID}
	if entry.ChatID != nil {
		chatQuery = `SELECT id FROM chats WHERE user_id=$1 AND id=$2 FOR UPDATE`
		args = append(args, *entry.ChatID)
	}
	err = pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		var chatID uuid.UUID
		if err := tx.QueryRow(ctx, chatQuery, args...).Scan(&chatID); err != nil {
			return wrapQueryError("resolve chat", err)
		}
		entry.ChatID = &chatID
		const insert = `INSERT INTO search_history (chat_id,user_id,query,filters,requested_limit,response,created_at)
 VALUES ($1,$2,$3,$4,$5,$6,clock_timestamp()) RETURNING id,created_at`
		return tx.QueryRow(ctx, insert, chatID, entry.UserID, entry.Query, filters, entry.RequestedLimit, response).Scan(&entry.ID, &entry.CreatedAt)
	})
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

	return scanEntry(r.pool.QueryRow(ctx, query, historyID, userID))
}

func scanEntry(row pgx.Row) (models.HistoryEntry, error) {
	var (
		entry       models.HistoryEntry
		rawFilters  []byte
		rawResponse []byte
	)
	err := row.Scan(
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
	var deleted bool
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		const find = `
            SELECT c.id FROM chats c
            JOIN search_history h ON h.chat_id = c.id AND h.user_id = c.user_id
            WHERE h.id = $1 AND h.user_id = $2 FOR UPDATE OF c`
		var chatID uuid.UUID
		err := tx.QueryRow(ctx, find, historyID, userID).Scan(&chatID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM search_history WHERE id=$1 AND user_id=$2`, historyID, userID)
		if err != nil {
			return err
		}
		deleted = tag.RowsAffected() > 0
		_, err = tx.Exec(ctx, `
            DELETE FROM chats WHERE id = $1 AND user_id = $2 AND NOT EXISTS (
                SELECT 1 FROM search_history WHERE chat_id = $1 AND user_id = $2
            )`, chatID, userID)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("delete history entry: %w", err)
	}
	return deleted, nil
}

var _ ports.HistoryRepository = (*History)(nil)
