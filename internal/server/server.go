package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Zulut30/local-telegram-client/internal/botapi"
	"github.com/Zulut30/local-telegram-client/internal/config"
	"github.com/Zulut30/local-telegram-client/internal/events"
	"github.com/Zulut30/local-telegram-client/internal/media"
	"github.com/Zulut30/local-telegram-client/internal/sim"
	"github.com/Zulut30/local-telegram-client/internal/store"
	tracing "github.com/Zulut30/local-telegram-client/internal/trace"
	"github.com/Zulut30/local-telegram-client/internal/webhook"
	"github.com/Zulut30/local-telegram-client/internal/webui"
)

func New(cfg config.Config, logger *slog.Logger) http.Handler {
	return NewWithStore(cfg, logger, store.NewMemory())
}

func NewWithStore(cfg config.Config, logger *slog.Logger, st store.Store) http.Handler {
	hub := events.NewHub(cfg.BufferSize)
	recorder := tracing.NewRecorder(cfg.BufferSize, hub)
	webhooks := webhook.New(logger, recorder)
	mediaStore := media.NewMemory()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	simHandler := sim.New(st, logger, hub, mediaStore, recorder, webhooks)
	mux.HandleFunc("POST /_sim/inject", simHandler.Inject)
	mux.HandleFunc("GET /_sim/state", simHandler.State)
	mux.HandleFunc("POST /_sim/reset", simHandler.Reset)
	mux.HandleFunc("GET /_sim/coverage", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": botapi.Coverage(cfg.EffectiveAPIMode())})
	})
	mux.HandleFunc("GET /_sim/traces", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": recorder.Snapshot()})
	})
	mux.HandleFunc("POST /_sim/traces/reset", func(w http.ResponseWriter, _ *http.Request) {
		recorder.Reset()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": true})
	})
	mux.HandleFunc("GET /_sim/file/{file_id}", func(w http.ResponseWriter, r *http.Request) {
		serveMediaFile(w, r, mediaStore)
	})
	mux.Handle("GET /_sim/events", hub)
	mux.Handle("GET /", webui.Handler())

	botHandler := botapi.New(cfg, st, logger, hub, mediaStore, recorder, webhooks)
	var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/bot") {
			botHandler.ServeHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
	handler = accessTokenMiddleware(cfg, handler)
	handler = loggingMiddleware(logger, handler)
	handler = recoverMiddleware(logger, handler)
	return handler
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func serveMediaFile(w http.ResponseWriter, r *http.Request, mediaStore media.Store) {
	fileID := r.PathValue("file_id")
	if fileID == "" {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	file, err := mediaStore.Get(r.Context(), fileID)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	if file.ContentType != "" {
		w.Header().Set("Content-Type", file.ContentType)
	}
	if file.FileName != "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", file.FileName))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(file.Data)
}

func loggingMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func recoverMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				logger.Error("panic recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"panic", fmt.Sprint(value),
					"stack", string(debug.Stack()),
				)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func accessTokenMiddleware(cfg config.Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.Mode != config.ModeRemote || r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/bot") {
			next.ServeHTTP(w, r)
			return
		}
		if cfg.Token == "" || requestToken(r) != cfg.Token {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requestToken(r *http.Request) string {
	if token := r.Header.Get("Authorization"); len(token) > len("Bearer ") && token[:len("Bearer ")] == "Bearer " {
		return token[len("Bearer "):]
	}
	if token := r.Header.Get("X-Sim-Token"); token != "" {
		return token
	}
	return r.URL.Query().Get("token")
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (rec *statusRecorder) WriteHeader(status int) {
	rec.status = status
	rec.ResponseWriter.WriteHeader(status)
}

func (rec *statusRecorder) Flush() {
	flusher, ok := rec.ResponseWriter.(http.Flusher)
	if !ok {
		return
	}
	flusher.Flush()
}
