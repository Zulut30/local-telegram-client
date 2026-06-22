package botapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Zulut30/local-telegram-client/internal/config"
	"github.com/Zulut30/local-telegram-client/internal/store"
	"github.com/Zulut30/local-telegram-client/internal/tg"
)

type Handler struct {
	cfg    config.Config
	store  store.Store
	logger *slog.Logger
	bot    tg.User
}

func New(cfg config.Config, st store.Store, logger *slog.Logger) *Handler {
	return &Handler{
		cfg:    cfg,
		store:  st,
		logger: logger,
		bot:    BotUser(cfg.BotToken),
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
		writeError(w, http.StatusBadRequest, 400, err.Error())
		return
	}

	switch method {
	case "getMe":
		writeOK(w, h.bot)
	case "getUpdates":
		h.handleGetUpdates(w, r, params)
	case "sendMessage":
		h.handleSendMessage(w, r, params)
	default:
		h.logger.Info("unimplemented bot api method", "method", method)
		writeOK(w, true)
	}
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
	writeOK(w, updates)
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
	writeOK(w, msg)
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
