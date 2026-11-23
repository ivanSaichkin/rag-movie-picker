package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
)

// ChatHandler serves saved conversations for the authenticated user.
type ChatHandler struct {
	chats ports.ChatRepository
}

// NewChatHandler builds a handler backed by the chat repository.
func NewChatHandler(chats ports.ChatRepository) *ChatHandler {
	return &ChatHandler{chats: chats}
}

type chatResponse struct {
	ID        uuid.UUID `json:"chat_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func chatDTO(chat models.Chat) chatResponse {
	return chatResponse{ID: chat.ID, Title: chat.Title, CreatedAt: chat.CreatedAt, UpdatedAt: chat.UpdatedAt}
}

func chatOwner(c *gin.Context) (uuid.UUID, bool) {
	userID, ok := middleware.UserIDFrom(c)
	if !ok {
		dto.Abort(c, dto.CodeUnauthenticated, "Требуется авторизация.")
	}
	return userID, ok
}

// List returns the current user's chat summaries.
func (h *ChatHandler) List(c *gin.Context) {
	userID, ok := chatOwner(c)
	if !ok {
		return
	}
	chats, err := h.chats.ListChats(c.Request.Context(), userID)
	if err != nil {
		respondServiceError(c, err, "list chats", "Чат не найден.")
		return
	}
	items := make([]chatResponse, 0, len(chats))
	for _, chat := range chats {
		items = append(items, chatDTO(chat))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func chatParams(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	userID, ok := chatOwner(c)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		dto.Abort(c, dto.CodeValidationError, "Некорректный идентификатор чата.")
		return uuid.Nil, uuid.Nil, false
	}
	return userID, id, true
}

// Get returns a chat with its stored recommendations.
func (h *ChatHandler) Get(c *gin.Context) {
	userID, id, ok := chatParams(c)
	if !ok {
		return
	}
	chat, entries, err := h.chats.GetChat(c.Request.Context(), userID, id)
	if err != nil {
		respondServiceError(c, err, "get chat", "Чат не найден.")
		return
	}
	turns := make([]dto.HistoryEntryResponse, 0, len(entries))
	for _, entry := range entries {
		turns = append(turns, dto.NewHistoryEntryResponse(entry))
	}
	c.JSON(http.StatusOK, struct {
		chatResponse
		Turns []dto.HistoryEntryResponse `json:"turns"`
	}{chatDTO(chat), turns})
}

// Delete removes a chat and all its turns.
func (h *ChatHandler) Delete(c *gin.Context) {
	userID, id, ok := chatParams(c)
	if !ok {
		return
	}
	if err := h.chats.DeleteChat(c.Request.Context(), userID, id); err != nil {
		respondServiceError(c, err, "delete chat", "Чат не найден.")
		return
	}
	c.Status(http.StatusNoContent)
}
