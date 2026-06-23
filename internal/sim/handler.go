package sim

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/Zulut30/local-telegram-client/internal/events"
	"github.com/Zulut30/local-telegram-client/internal/store"
	"github.com/Zulut30/local-telegram-client/internal/tg"
	tracing "github.com/Zulut30/local-telegram-client/internal/trace"
	"github.com/Zulut30/local-telegram-client/internal/webhook"
)

type Handler struct {
	store    store.Store
	logger   *slog.Logger
	hub      *events.Hub
	recorder *tracing.Recorder
	webhooks *webhook.Manager
}

type injectRequest struct {
	Type      string `json:"type"`
	ChatID    int64  `json:"chat_id"`
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	Text      string `json:"text"`
	MessageID int64  `json:"message_id"`
	Data      string `json:"data"`
}

type response struct {
	OK     bool `json:"ok"`
	Result any  `json:"result,omitempty"`
}

func New(st store.Store, logger *slog.Logger, hub *events.Hub, recorder *tracing.Recorder, webhooks *webhook.Manager) *Handler {
	return &Handler{store: st, logger: logger, hub: hub, recorder: recorder, webhooks: webhooks}
}

func (h *Handler) Inject(w http.ResponseWriter, r *http.Request) {
	var req injectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	switch req.Type {
	case "", "message", "text":
		h.injectText(w, r, req)
	case "callback_query", "callback":
		h.injectCallback(w, r, req)
	default:
		writeError(w, http.StatusBadRequest, "unsupported injection type")
	}
}

func (h *Handler) injectText(w http.ResponseWriter, r *http.Request, req injectRequest) {
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	update, err := h.store.InjectText(r.Context(), store.TextInput{
		ChatID:    req.ChatID,
		UserID:    req.UserID,
		Username:  req.Username,
		FirstName: req.FirstName,
		Text:      req.Text,
	})
	if err != nil {
		h.logger.Error("inject text", "error", err)
		writeError(w, http.StatusInternalServerError, "inject text")
		return
	}
	if h.hub != nil && update.Message != nil {
		h.hub.Broadcast("message", map[string]any{"op": "created", "message": update.Message})
	}
	if !h.deliverWebhook(w, r, update) {
		return
	}
	writeJSON(w, http.StatusOK, response{OK: true, Result: update})
}

func (h *Handler) injectCallback(w http.ResponseWriter, r *http.Request, req injectRequest) {
	if req.Data == "" {
		writeError(w, http.StatusBadRequest, "data is required")
		return
	}

	update, err := h.store.InjectCallback(r.Context(), store.CallbackInput{
		ChatID:    req.ChatID,
		MessageID: req.MessageID,
		UserID:    req.UserID,
		Username:  req.Username,
		FirstName: req.FirstName,
		Data:      req.Data,
	})
	if err != nil {
		if errors.Is(err, store.ErrMessageNotFound) {
			writeError(w, http.StatusBadRequest, "message not found")
			return
		}
		h.logger.Error("inject callback", "error", err)
		writeError(w, http.StatusInternalServerError, "inject callback")
		return
	}
	if !h.deliverWebhook(w, r, update) {
		return
	}
	writeJSON(w, http.StatusOK, response{OK: true, Result: update})
}

func (h *Handler) deliverWebhook(w http.ResponseWriter, r *http.Request, update tg.Update) bool {
	if h.webhooks == nil {
		return true
	}
	delivered, err := h.webhooks.Deliver(r.Context(), update)
	if err != nil {
		writeError(w, http.StatusBadGateway, "webhook delivery failed")
		return false
	}
	if delivered {
		if err := h.store.AckUpdates(r.Context(), update.UpdateID+1); err != nil {
			h.logger.Error("ack webhook update", "error", err)
			writeError(w, http.StatusInternalServerError, "ack webhook update")
			return false
		}
	}
	return true
}

func (h *Handler) State(w http.ResponseWriter, r *http.Request) {
	state, err := h.store.State(r.Context())
	if err != nil {
		h.logger.Error("read state", "error", err)
		writeError(w, http.StatusInternalServerError, "read state")
		return
	}
	writeJSON(w, http.StatusOK, response{OK: true, Result: state})
}

func (h *Handler) Reset(w http.ResponseWriter, r *http.Request) {
	if h.webhooks != nil {
		h.webhooks.Delete()
	}
	if err := h.store.Reset(r.Context()); err != nil {
		h.logger.Error("reset store", "error", err)
		writeError(w, http.StatusInternalServerError, "reset store")
		return
	}
	if h.recorder != nil {
		h.recorder.Reset()
	}
	writeJSON(w, http.StatusOK, response{OK: true, Result: true})
}

func writeError(w http.ResponseWriter, status int, description string) {
	writeJSON(w, status, map[string]any{"ok": false, "description": description})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
