package botapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Zulut30/local-telegram-client/internal/config"
	"github.com/Zulut30/local-telegram-client/internal/events"
	"github.com/Zulut30/local-telegram-client/internal/store"
	"github.com/Zulut30/local-telegram-client/internal/tg"
	tracing "github.com/Zulut30/local-telegram-client/internal/trace"
	"github.com/Zulut30/local-telegram-client/internal/webhook"
)

type Handler struct {
	cfg      config.Config
	store    store.Store
	logger   *slog.Logger
	bot      tg.User
	hub      *events.Hub
	recorder *tracing.Recorder
	webhooks *webhook.Manager
}

func New(cfg config.Config, st store.Store, logger *slog.Logger, hub *events.Hub, recorder *tracing.Recorder, webhooks *webhook.Manager) *Handler {
	return &Handler{
		cfg:      cfg,
		store:    st,
		logger:   logger,
		bot:      BotUser(cfg.BotToken),
		hub:      hub,
		recorder: recorder,
		webhooks: webhooks,
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token, method, ok := parseBotPath(r.URL.Path)
	if !ok {
		writeError(w, http.StatusNotFound, 404, "bot API path not found")
		return
	}
	if token != h.cfg.BotToken {
		writeError(w, http.StatusUnauthorized, 401, "unauthorized bot token")
		return
	}

	params, err := parseParams(r)
	if err != nil {
		if shouldTrace(method) {
			h.recordCall(method, params, 0, responseMeta{
				httpStatus: http.StatusBadRequest,
				ok:         false,
				errorCode:  400,
				errorDesc:  err.Error(),
			})
		}
		writeError(w, http.StatusBadRequest, 400, err.Error())
		return
	}

	if shouldTrace(method) {
		h.serveTraced(w, r, method, params)
		return
	}
	h.dispatch(w, r, method, params)
}

func (h *Handler) dispatch(w http.ResponseWriter, r *http.Request, method string, params parameters) {
	switch method {
	case "getMe":
		writeOK(w, h.bot)
	case "getUpdates":
		h.handleGetUpdates(w, r, params)
	case "setWebhook":
		h.handleSetWebhook(w, r, params)
	case "deleteWebhook":
		h.handleDeleteWebhook(w, r, params)
	case "getWebhookInfo":
		h.handleGetWebhookInfo(w)
	case "sendMessage":
		h.handleSendMessage(w, r, params)
	case "sendPhoto":
		h.handleSendPhoto(w, r, params)
	case "editMessageText":
		h.handleEditMessageText(w, r, params)
	case "editMessageReplyMarkup":
		h.handleEditMessageReplyMarkup(w, r, params)
	case "deleteMessage":
		h.handleDeleteMessage(w, r, params)
	case "answerCallbackQuery":
		h.handleAnswerCallbackQuery(w, params)
	default:
		params["_sim_unimplemented"] = true
		h.logger.Info("unimplemented bot api method", "method", method)
		writeOK(w, true)
	}
}

func (h *Handler) serveTraced(w http.ResponseWriter, r *http.Request, method string, params parameters) {
	capture := newCaptureResponseWriter()
	start := time.Now()
	h.dispatch(capture, r, method, params)
	latency := time.Since(start)
	meta := capture.Meta()
	capture.FlushTo(w)
	h.recordCall(method, params, latency, meta)
}

func (h *Handler) recordCall(method string, params parameters, latency time.Duration, meta responseMeta) {
	if h.recorder == nil {
		return
	}
	chatID := params.ChatID()
	callbackQueryID, _ := params.String("callback_query_id", "")
	h.recorder.RecordCall(chatID, callbackQueryID, tracing.OutboundCall{
		Method:     method,
		Params:     params.TraceParams(),
		HTTPStatus: meta.httpStatus,
		OK:         meta.ok,
		ErrorCode:  meta.errorCode,
		ErrorDesc:  meta.errorDesc,
		LatencyMS:  latency.Milliseconds(),
	})
}

func shouldTrace(method string) bool {
	return method != "getMe" && method != "getUpdates"
}

func BotUser(token string) tg.User {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(token))
	username := sanitizeUsername(token)
	if username == "" {
		username = "sim_bot"
	}
	if !strings.HasSuffix(username, "_bot") {
		username += "_bot"
	}

	return tg.User{
		ID:        int64(1_000_000_000 + hash.Sum32()%900_000_000),
		IsBot:     true,
		FirstName: "Sim Bot",
		Username:  username,
	}
}

func (h *Handler) handleGetUpdates(w http.ResponseWriter, r *http.Request, params parameters) {
	if h.webhooks != nil && h.webhooks.Active() {
		writeError(w, http.StatusConflict, 409, "Conflict: can't use getUpdates method while webhook is active")
		return
	}
	if h.recorder != nil {
		h.recorder.FlushOpen()
	}

	offset, err := params.Int64("offset", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "offset must be an integer")
		return
	}
	limit, err := params.Int("limit", 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "limit must be an integer")
		return
	}
	timeoutSeconds, err := params.Int("timeout", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "timeout must be an integer")
		return
	}
	if timeoutSeconds < 0 {
		timeoutSeconds = 0
	}

	updates, err := h.store.GetUpdates(r.Context(), offset, limit, time.Duration(timeoutSeconds)*time.Second)
	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, err.Error())
		return
	}
	if h.recorder != nil {
		h.recorder.OpenForUpdates(updates)
	}
	writeOK(w, updates)
}

func (h *Handler) handleSetWebhook(w http.ResponseWriter, r *http.Request, params parameters) {
	rawURL, _ := params.String("url", "")
	if rawURL == "" {
		writeError(w, http.StatusBadRequest, 400, "url is required")
		return
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		writeError(w, http.StatusBadRequest, 400, "url must be absolute")
		return
	}
	switch parsed.Scheme {
	case "http", "https":
	default:
		writeError(w, http.StatusBadRequest, 400, "url scheme must be http or https")
		return
	}
	dropPending, err := params.Bool("drop_pending_updates", false)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "drop_pending_updates must be boolean")
		return
	}

	if h.webhooks != nil {
		h.webhooks.Set(rawURL)
	}
	if dropPending {
		if err := h.store.AckUpdates(r.Context(), 1<<62); err != nil {
			writeError(w, http.StatusInternalServerError, 500, err.Error())
			return
		}
	}
	writeOK(w, true)
}

func (h *Handler) handleDeleteWebhook(w http.ResponseWriter, r *http.Request, params parameters) {
	dropPending, err := params.Bool("drop_pending_updates", false)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "drop_pending_updates must be boolean")
		return
	}
	if h.webhooks != nil {
		h.webhooks.Delete()
	}
	if dropPending {
		if err := h.store.AckUpdates(r.Context(), 1<<62); err != nil {
			writeError(w, http.StatusInternalServerError, 500, err.Error())
			return
		}
	}
	writeOK(w, true)
}

func (h *Handler) handleGetWebhookInfo(w http.ResponseWriter) {
	if h.webhooks == nil {
		writeOK(w, webhook.Info{})
		return
	}
	writeOK(w, h.webhooks.Info())
}

func (h *Handler) handleSendMessage(w http.ResponseWriter, r *http.Request, params parameters) {
	chatID, err := params.Int64("chat_id", 0)
	if err != nil || chatID == 0 {
		writeError(w, http.StatusBadRequest, 400, "chat_id is required")
		return
	}
	text, _ := params.String("text", "")
	if text == "" {
		writeError(w, http.StatusBadRequest, 400, "text is required")
		return
	}
	replyToMessageID, err := params.Int64("reply_to_message_id", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "reply_to_message_id must be an integer")
		return
	}
	replyMarkup, err := params.RawJSON("reply_markup")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "reply_markup must be valid JSON")
		return
	}

	msg, err := h.store.SaveBotMessage(r.Context(), store.BotMessageInput{
		From:             h.bot,
		ChatID:           chatID,
		Text:             text,
		ReplyMarkup:      replyMarkup,
		ReplyToMessageID: replyToMessageID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, err.Error())
		return
	}
	if h.hub != nil {
		h.hub.Broadcast("message", map[string]any{"op": "created", "message": msg})
	}
	writeOK(w, msg)
}

func (h *Handler) handleSendPhoto(w http.ResponseWriter, r *http.Request, params parameters) {
	chatID, err := params.Int64("chat_id", 0)
	if err != nil || chatID == 0 {
		writeError(w, http.StatusBadRequest, 400, "chat_id is required")
		return
	}
	photo, _ := params.String("photo", "")
	if photo == "" {
		writeError(w, http.StatusBadRequest, 400, "photo is required")
		return
	}
	caption, _ := params.String("caption", "")
	replyToMessageID, err := params.Int64("reply_to_message_id", 0)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "reply_to_message_id must be an integer")
		return
	}
	replyMarkup, err := params.RawJSON("reply_markup")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "reply_markup must be valid JSON")
		return
	}

	msg, err := h.store.SaveBotMessage(r.Context(), store.BotMessageInput{
		From:             h.bot,
		ChatID:           chatID,
		Caption:          caption,
		PhotoURL:         photo,
		ReplyMarkup:      replyMarkup,
		ReplyToMessageID: replyToMessageID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, err.Error())
		return
	}
	h.broadcastMessage("created", msg)
	writeOK(w, msg)
}

func (h *Handler) handleEditMessageText(w http.ResponseWriter, r *http.Request, params parameters) {
	chatID, err := params.Int64("chat_id", 0)
	if err != nil || chatID == 0 {
		writeError(w, http.StatusBadRequest, 400, "chat_id is required")
		return
	}
	messageID, err := params.Int64("message_id", 0)
	if err != nil || messageID == 0 {
		writeError(w, http.StatusBadRequest, 400, "message_id is required")
		return
	}
	text, _ := params.String("text", "")
	if text == "" {
		writeError(w, http.StatusBadRequest, 400, "text is required")
		return
	}
	replyMarkup, err := params.RawJSON("reply_markup")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "reply_markup must be valid JSON")
		return
	}

	msg, err := h.store.EditMessageText(r.Context(), store.EditMessageTextInput{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        text,
		ReplyMarkup: replyMarkup,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	h.broadcastMessage("edited", msg)
	writeOK(w, msg)
}

func (h *Handler) handleEditMessageReplyMarkup(w http.ResponseWriter, r *http.Request, params parameters) {
	chatID, err := params.Int64("chat_id", 0)
	if err != nil || chatID == 0 {
		writeError(w, http.StatusBadRequest, 400, "chat_id is required")
		return
	}
	messageID, err := params.Int64("message_id", 0)
	if err != nil || messageID == 0 {
		writeError(w, http.StatusBadRequest, 400, "message_id is required")
		return
	}
	replyMarkup, err := params.RawJSON("reply_markup")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "reply_markup must be valid JSON")
		return
	}

	msg, err := h.store.EditMessageReplyMarkup(r.Context(), store.EditMessageReplyMarkupInput{
		ChatID:      chatID,
		MessageID:   messageID,
		ReplyMarkup: replyMarkup,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	h.broadcastMessage("edited", msg)
	writeOK(w, msg)
}

func (h *Handler) handleDeleteMessage(w http.ResponseWriter, r *http.Request, params parameters) {
	chatID, err := params.Int64("chat_id", 0)
	if err != nil || chatID == 0 {
		writeError(w, http.StatusBadRequest, 400, "chat_id is required")
		return
	}
	messageID, err := params.Int64("message_id", 0)
	if err != nil || messageID == 0 {
		writeError(w, http.StatusBadRequest, 400, "message_id is required")
		return
	}

	msg, err := h.store.DeleteMessage(r.Context(), chatID, messageID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	h.broadcastMessage("deleted", msg)
	writeOK(w, true)
}

func (h *Handler) handleAnswerCallbackQuery(w http.ResponseWriter, params parameters) {
	callbackID, _ := params.String("callback_query_id", "")
	if callbackID == "" {
		writeError(w, http.StatusBadRequest, 400, "callback_query_id is required")
		return
	}
	text, _ := params.String("text", "")
	showAlert, err := params.Bool("show_alert", false)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "show_alert must be boolean")
		return
	}
	if h.hub != nil {
		h.hub.Broadcast("callback_answer", map[string]any{
			"callback_query_id": callbackID,
			"text":              text,
			"show_alert":        showAlert,
		})
	}
	writeOK(w, true)
}

func (h *Handler) broadcastMessage(op string, msg tg.Message) {
	if h.hub == nil {
		return
	}
	h.hub.Broadcast("message", map[string]any{"op": op, "message": msg})
}

func writeStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrMessageNotFound) {
		writeError(w, http.StatusBadRequest, 400, "message not found")
		return
	}
	writeError(w, http.StatusInternalServerError, 500, err.Error())
}

func parseBotPath(path string) (string, string, bool) {
	rest := strings.TrimPrefix(path, "/bot")
	if rest == path || rest == "" {
		return "", "", false
	}

	slash := strings.IndexByte(rest, '/')
	if slash <= 0 || slash == len(rest)-1 {
		return "", "", false
	}
	return rest[:slash], rest[slash+1:], true
}

func sanitizeUsername(token string) string {
	var b strings.Builder
	for _, r := range token {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		case r == '_':
			b.WriteRune(r)
		case b.Len() > 0:
			b.WriteByte('_')
		}
		if b.Len() >= 24 {
			break
		}
	}
	return strings.Trim(b.String(), "_")
}

type apiResponse struct {
	OK          bool   `json:"ok"`
	Result      any    `json:"result,omitempty"`
	ErrorCode   int    `json:"error_code,omitempty"`
	Description string `json:"description,omitempty"`
}

func writeOK(w http.ResponseWriter, result any) {
	writeJSON(w, http.StatusOK, apiResponse{OK: true, Result: result})
}

func writeError(w http.ResponseWriter, status, code int, description string) {
	writeJSON(w, status, apiResponse{OK: false, ErrorCode: code, Description: description})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type responseMeta struct {
	httpStatus int
	ok         bool
	errorCode  int
	errorDesc  string
}

type captureResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newCaptureResponseWriter() *captureResponseWriter {
	return &captureResponseWriter{header: make(http.Header)}
}

func (w *captureResponseWriter) Header() http.Header {
	return w.header
}

func (w *captureResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
}

func (w *captureResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}

func (w *captureResponseWriter) FlushTo(dst http.ResponseWriter) {
	for key, values := range w.header {
		for _, value := range values {
			dst.Header().Add(key, value)
		}
	}
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	dst.WriteHeader(status)
	_, _ = dst.Write(w.body.Bytes())
}

func (w *captureResponseWriter) Meta() responseMeta {
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	meta := responseMeta{httpStatus: status, ok: status < http.StatusBadRequest}
	var api apiResponse
	if err := json.Unmarshal(w.body.Bytes(), &api); err == nil {
		meta.ok = api.OK
		meta.errorCode = api.ErrorCode
		meta.errorDesc = api.Description
	}
	return meta
}

type parameters map[string]any

func parseParams(r *http.Request) (parameters, error) {
	params := parameters{}
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			params[key] = values[len(values)-1]
		}
	}

	if r.Body == nil || r.Body == http.NoBody {
		return params, nil
	}

	contentType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch contentType {
	case "application/json":
		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		var body map[string]any
		if err := decoder.Decode(&body); err != nil {
			if errors.Is(err, io.EOF) {
				return params, nil
			}
			return nil, fmt.Errorf("decode JSON body: %w", err)
		}
		for key, value := range body {
			params[key] = value
		}
	case "application/x-www-form-urlencoded", "":
		if err := r.ParseForm(); err != nil {
			return nil, fmt.Errorf("parse form body: %w", err)
		}
		for key, values := range r.PostForm {
			if len(values) > 0 {
				params[key] = values[len(values)-1]
			}
		}
	case "multipart/form-data":
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return nil, fmt.Errorf("parse multipart body: %w", err)
		}
		for key, values := range r.MultipartForm.Value {
			if len(values) > 0 {
				params[key] = values[len(values)-1]
			}
		}
	default:
		return nil, fmt.Errorf("unsupported content type %q", contentType)
	}

	return params, nil
}

func (p parameters) String(key, fallback string) (string, bool) {
	value, ok := p[key]
	if !ok || value == nil {
		return fallback, false
	}
	switch typed := value.(type) {
	case string:
		return typed, true
	case json.Number:
		return typed.String(), true
	default:
		return fmt.Sprint(typed), true
	}
}

func (p parameters) Int(key string, fallback int) (int, error) {
	value, ok := p.Int64(key, int64(fallback))
	return int(value), ok
}

func (p parameters) Int64(key string, fallback int64) (int64, error) {
	value, ok := p[key]
	if !ok || value == nil {
		return fallback, nil
	}
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case float64:
		return int64(typed), nil
	case json.Number:
		return typed.Int64()
	case string:
		if typed == "" {
			return fallback, nil
		}
		return strconv.ParseInt(typed, 10, 64)
	default:
		return 0, fmt.Errorf("%s must be an integer", key)
	}
}

func (p parameters) Bool(key string, fallback bool) (bool, error) {
	value, ok := p[key]
	if !ok || value == nil {
		return fallback, nil
	}
	switch typed := value.(type) {
	case bool:
		return typed, nil
	case string:
		if typed == "" {
			return fallback, nil
		}
		return strconv.ParseBool(typed)
	default:
		return false, fmt.Errorf("%s must be boolean", key)
	}
}

func (p parameters) ChatID() *int64 {
	chatID, err := p.Int64("chat_id", 0)
	if err != nil || chatID == 0 {
		return nil
	}
	return &chatID
}

func (p parameters) TraceParams() map[string]any {
	out := make(map[string]any, len(p))
	for key, value := range p {
		out[key] = value
	}
	return out
}

func (p parameters) RawJSON(key string) (json.RawMessage, error) {
	value, ok := p[key]
	if !ok || value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil, nil
		}
		if !json.Valid([]byte(typed)) {
			return nil, fmt.Errorf("%s must be valid JSON", key)
		}
		return json.RawMessage(typed), nil
	case json.RawMessage:
		return typed, nil
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return nil, err
		}
		return raw, nil
	}
}
