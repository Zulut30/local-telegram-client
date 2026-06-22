package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Zulut30/local-telegram-client/internal/config"
)

func TestHealthz(t *testing.T) {
	handler := New(config.Config{Mode: config.ModeLocal, BotToken: "bot", BufferSize: 1}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Body.String() != `{"ok":true}` {
		t.Fatalf("body = %q, want health JSON", rec.Body.String())
	}
}

func TestRemoteModeRequiresTokenForUI(t *testing.T) {
	cfg := config.Config{Mode: config.ModeRemote, Token: "secret", BotToken: "bot", BufferSize: 1}
	handler := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status without token = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest(http.MethodGet, "/?token=secret", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status with token = %d, want %d", rec.Code, http.StatusOK)
	}
}
