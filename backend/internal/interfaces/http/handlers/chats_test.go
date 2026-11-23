package handlers_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/models"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/domain/ports"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/dto"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/handlers"
	"github.com/ivanSaichkin/rag-movie-picker/backend/internal/interfaces/http/server/middleware"
	"github.com/ivanSaichkin/rag-movie-picker/backend/pkg/testutils"
)

type chatReader struct {
	ports.ChatRepository
	owner uuid.UUID
	id    uuid.UUID
}

func (r *chatReader) GetChat(_ context.Context, owner, id uuid.UUID) (models.Chat, []models.HistoryEntry, error) {
	r.owner, r.id = owner, id
	return models.Chat{ID: id, Title: "космос"}, []models.HistoryEntry{{Query: "космос", Response: models.Recommendation{Mode: "empty"}}}, nil
}

func TestChatDetailRestoresStoredTurn(t *testing.T) {
	reader := &chatReader{}
	engine := gin.New()
	engine.GET("/chats/:id", middleware.Authenticate(fakeIssuer{userID: testUserID}), handlers.NewChatHandler(reader).Get)
	id := uuid.New()
	route := "/chats/" + id.String()
	testutils.RequireStatus(t, testutils.Perform(t, engine, http.MethodGet, route, nil, nil), http.StatusUnauthorized)
	recorder := testutils.Perform(t, engine, http.MethodGet, route, nil, bearerHeader())
	testutils.RequireStatus(t, recorder, http.StatusOK)
	body := testutils.DecodeJSON[struct {
		ID    uuid.UUID                  `json:"chat_id"`
		Title string                     `json:"title"`
		Turns []dto.HistoryEntryResponse `json:"turns"`
	}](t, recorder)
	if reader.owner != testUserID || reader.id != id || body.ID != id || body.Title != "космос" || len(body.Turns) != 1 || body.Turns[0].Response.Mode != "empty" {
		t.Fatalf("owner=%v id=%v chat=%+v", reader.owner, reader.id, body)
	}
}
