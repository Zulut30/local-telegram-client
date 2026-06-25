package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Zulut30/local-telegram-client/internal/config"
)

func newTestHandler(cfg config.Config) http.Handler {
	return New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

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

func TestRemoteModeProtectsControlPlaneEndpoints(t *testing.T) {
	cfg := config.Config{Mode: config.ModeRemote, Token: "secret", BotToken: "bot", BufferSize: 1}
	handler := newTestHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/_sim/state", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("/_sim/state without token = %d, want 401", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/_sim/state", nil)
	req.Header.Set("X-Sim-Token", "secret")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/_sim/state with X-Sim-Token = %d, want 200", rec.Code)
	}
}

func TestVersionEndpointReportsBuildInfo(t *testing.T) {
	handler := newTestHandler(config.Config{Mode: config.ModeLocal, BotToken: "bot", APIMode: config.APIModeCompat, BufferSize: 1})

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/version status = %d, want 200", rec.Code)
	}

	var payload struct {
		OK     bool `json:"ok"`
		Result struct {
			Build         map[string]any `json:"build"`
			BotAPIVersion string         `json:"bot_api_version"`
			Mode          string         `json:"mode"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode /version: %v", err)
	}
	if !payload.OK || payload.Result.BotAPIVersion == "" || payload.Result.Mode != config.ModeLocal {
		t.Fatalf("unexpected /version payload: %s", rec.Body.String())
	}
}

func TestSecurityHeadersApplied(t *testing.T) {
	handler := newTestHandler(config.Config{Mode: config.ModeLocal, BotToken: "bot", BufferSize: 1})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}
