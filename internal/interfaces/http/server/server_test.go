package server_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"

	"github.com/ivanSaichkin/rag-movie-picker/internal/interfaces/http/server"
)

func TestRunReturnsErrorWhenAddressIsBusy(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve address: %v", err)
	}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Fatalf("close listener: %v", err)
		}
	})

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	instance := server.New(listener.Addr().String(), http.NotFoundHandler(), logger)

	if err := instance.Run(context.Background()); err == nil {
		t.Fatal("Run() must fail when the address is already in use")
	}
}
