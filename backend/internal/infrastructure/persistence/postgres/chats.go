package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/rules"
)

// CheckChat verifies ownership before a recommendation is requested.
func (r *History) CheckChat(ctx context.Context, userID, chatID uuid.UUID) error {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chats WHERE user_id=$1 AND id=$2)`, userID, chatID).Scan(&exists); err != nil {
		return fmt.Errorf("check chat: %w", err)
	}
	if !exists {
		return rules.ErrNotFound
	}
	return nil
}

// ListChats returns the user's chats, most recently updated first.
func (r *History) ListChats(ctx context.Context, userID uuid.UUID) ([]models.Chat, error) {
	// Derive the title from the earliest remaining turn so deleting a history
	// record also removes its query from the sidebar.
	const query = `SELECT chat_id, left(query,80), created_at, updated_at FROM (
     SELECT DISTINCT ON (chat_id) chat_id, query, created_at,
            max(created_at) OVER (PARTITION BY chat_id) AS updated_at
     FROM search_history WHERE user_id=$1 ORDER BY chat_id,created_at,id
 ) AS summary ORDER BY updated_at DESC, chat_id DESC`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list chats: %w", err)
	}
	defer rows.Close()
	chats := make([]models.Chat, 0)
	for rows.Next() {
		var chat models.Chat
		if err := rows.Scan(&chat.ID, &chat.Title, &chat.CreatedAt, &chat.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan chat: %w", err)
		}
		chats = append(chats, chat)
	}
	return chats, rows.Err()
}

// GetChat returns one chat and its turns in chronological order.
func (r *History) GetChat(ctx context.Context, userID, chatID uuid.UUID) (models.Chat, []models.HistoryEntry, error) {
	const query = `SELECT id,user_id,query,filters,requested_limit,response,created_at
 FROM search_history WHERE user_id=$1 AND chat_id=$2 ORDER BY created_at,id`
	rows, err := r.pool.Query(ctx, query, userID, chatID)
	if err != nil {
		return models.Chat{}, nil, fmt.Errorf("get chat: %w", err)
	}
	defer rows.Close()
	turns := make([]models.HistoryEntry, 0)
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return models.Chat{}, nil, err
		}
		turns = append(turns, entry)
	}
	if err := rows.Err(); err != nil {
		return models.Chat{}, nil, err
	}
	if len(turns) == 0 {
		return models.Chat{}, nil, rules.ErrNotFound
	}
	title := []rune(turns[0].Query)
	chat := models.Chat{ID: chatID, Title: string(title[:min(len(title), 80)]), CreatedAt: turns[0].CreatedAt, UpdatedAt: turns[len(turns)-1].CreatedAt}
	return chat, turns, nil
}

// DeleteChat removes a chat and lets PostgreSQL delete its turns.
func (r *History) DeleteChat(ctx context.Context, userID, chatID uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM chats WHERE user_id=$1 AND id=$2`, userID, chatID)
	if err != nil {
		return fmt.Errorf("delete chat: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return rules.ErrNotFound
	}
	return nil
}
