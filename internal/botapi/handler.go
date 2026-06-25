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
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Zulut30/local-telegram-client/internal/config"
	"github.com/Zulut30/local-telegram-client/internal/events"
	"github.com/Zulut30/local-telegram-client/internal/media"
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
	media    media.Store
	recorder *tracing.Recorder
	webhooks *webhook.Manager
}

func New(cfg config.Config, st store.Store, logger *slog.Logger, hub *events.Hub, mediaStore media.Store, recorder *tracing.Recorder, webhooks *webhook.Manager) *Handler {
	return &Handler{
		cfg:      cfg,
		store:    st,
		logger:   logger,
		bot:      BotUser(cfg.BotToken),
		hub:      hub,
		media:    mediaStore,
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
	canonicalMethod, knownMethod := canonicalBotAPIMethod(method)
	if !knownMethod {
		writeError(w, http.StatusNotFound, 404, "method not found")
		return
	}
	method = canonicalMethod

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

	if h.cfg.EffectiveAPIMode() == config.APIModeStrict && !StrictSupports(method) {
		description := strictUnsupportedDescription(method)
		if shouldTrace(method) {
			h.recordCall(method, params, 0, responseMeta{
				httpStatus: http.StatusNotImplemented,
				ok:         false,
				errorCode:  http.StatusNotImplemented,
				errorDesc:  description,
			})
		}
		writeError(w, http.StatusNotImplemented, http.StatusNotImplemented, description)
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
	case "getFile":
		h.handleGetFile(w, r, params)
	case "sendMessage":
		h.handleSendMessage(w, r, params)
	case "sendPhoto":
		h.handleSendPhoto(w, r, params)
	case "sendRichMessage":
		h.handleSendRichMessage(w, r, params)
	case "sendMessageDraft", "sendRichMessageDraft":
		h.handleSendDraft(w, params, method)
	case "sendChatAction":
		h.handleSendChatAction(w, params)
	case "sendMediaGroup":
		h.handleSendMediaGroup(w, r, params)
	case "copyMessage":
		h.handleCopyMessage(w, r, params)
	case "copyMessages", "forwardMessages":
		h.handleMessageIDList(w, method, params)
	case "editMessageText":
		h.handleEditMessageText(w, r, params)
	case "editMessageCaption":
		h.handleEditMessageCaption(w, r, params)
	case "editMessageMedia":
		h.handleEditMessageMedia(w, r, params)
	case "editMessageReplyMarkup":
		h.handleEditMessageReplyMarkup(w, r, params)
	case "deleteMessage":
		h.handleDeleteMessage(w, r, params)
	case "deleteMessages":
		h.handleDeleteMessages(w, r, params)
	case "answerCallbackQuery":
		h.handleAnswerCallbackQuery(w, params)
	case "getCustomEmojiStickers":
		h.handleGetCustomEmojiStickers(w, params)
	default:
		if isGenericSendMessageMethod(method) {
			h.handleGenericSendMessage(w, r, method, params)
			return
		}
		h.handleKnownMethodStub(w, method, params)
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
	switch method {
	case "getMe", "getUpdates":
		return false
	default:
		return true
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
	allowed, _ := params.StringSlice("allowed_updates")
	updates = filterAllowedUpdates(updates, allowed)
	if h.recorder != nil {
		h.recorder.OpenForUpdates(updates)
	}
	writeOK(w, updates)
}

// filterAllowedUpdates honors the getUpdates allowed_updates parameter. An empty
// or nil list means "all update types" (Telegram's documented default).
func filterAllowedUpdates(updates []tg.Update, allowed []string) []tg.Update {
	if len(allowed) == 0 {
		return updates
	}
	want := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		want[name] = struct{}{}
	}
	out := make([]tg.Update, 0, len(updates))
	for _, update := range updates {
		if _, ok := want[updateType(update)]; ok {
			out = append(out, update)
		}
	}
	return out
}

func updateType(update tg.Update) string {
	switch {
	case update.Message != nil:
		return "message"
	case update.CallbackQuery != nil:
		return "callback_query"
	default:
		return ""
	}
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
	if h.cfg.Mode == config.ModeRemote && !h.cfg.AllowPrivateWebhooks && hostResolvesToPrivate(parsed.Hostname()) {
		writeError(w, http.StatusBadRequest, 400, "url resolves to a private or loopback address; pass --allow-private-webhooks to override")
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

// hostResolvesToPrivate reports whether a webhook host points at a private,
// loopback, link-local, or unspecified address — a basic SSRF guard for remote
// mode. IP literals are checked directly; hostnames are resolved best-effort
// (an unresolvable host is treated as safe, since delivery will simply fail).
func hostResolvesToPrivate(host string) bool {
	if host == "" {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return isPrivateIP(ip)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			return true
		}
	}
	return false
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

func (h *Handler) handleGetWebhookInfo(w http.ResponseWriter) {
	if h.webhooks == nil {
		writeOK(w, webhook.Info{})
		return
	}
	writeOK(w, h.webhooks.Info())
}

func (h *Handler) handleSendMessage(w http.ResponseWriter, r *http.Request, params parameters) {
	chatID, err := params.ChatIDValue("chat_id", 0)
	if err != nil || chatID == 0 {
		writeError(w, http.StatusBadRequest, 400, "chat_id is required")
		return
	}
	text, _ := params.String("text", "")
	if text == "" {
		writeError(w, http.StatusBadRequest, 400, "text is required")
		return
	}
	replyToMessageID, err := params.ReplyToMessageID()
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "reply_to_message_id must be an integer")
		return
	}
	replyMarkup, err := params.RawJSON("reply_markup")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "reply_markup must be valid JSON")
		return
	}
	entities, err := params.Entities("entities")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "entities must be valid JSON")
		return
	}
	parseMode, _ := params.String("parse_mode", "")
	linkPreview, err := params.RawJSON("link_preview_options")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "link_preview_options must be valid JSON")
		return
	}
	threadID, _ := params.Int64("message_thread_id", 0)
	businessConnID, _ := params.String("business_connection_id", "")

	msg, err := h.store.SaveBotMessage(r.Context(), store.BotMessageInput{
		From:                 h.bot,
		ChatID:               chatID,
		MessageThreadID:      threadID,
		BusinessConnectionID: businessConnID,
		LinkPreviewOptions:   linkPreview,
		Text:                 text,
		Entities:             entities,
		ParseMode:            parseMode,
		ReplyMarkup:          replyMarkup,
		ReplyToMessageID:     replyToMessageID,
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

func (h *Handler) handleSendChatAction(w http.ResponseWriter, params parameters) {
	chatID, err := params.ChatIDValue("chat_id", 0)
	if err != nil || chatID == 0 {
		writeError(w, http.StatusBadRequest, 400, "chat_id is required")
		return
	}
	action, _ := params.String("action", "")
	if !validChatAction(action) {
		writeError(w, http.StatusBadRequest, 400, "action is invalid")
		return
	}
	if h.hub != nil {
		h.hub.Broadcast("chat_action", map[string]any{
			"chat_id": chatID,
			"action":  action,
			"from":    h.bot,
			"until":   time.Now().Add(5 * time.Second).UnixMilli(),
		})
	}
	writeOK(w, true)
}

func (h *Handler) handleSendPhoto(w http.ResponseWriter, r *http.Request, params parameters) {
	chatID, err := params.ChatIDValue("chat_id", 0)
	if err != nil || chatID == 0 {
		writeError(w, http.StatusBadRequest, 400, "chat_id is required")
		return
	}
	photoRef, photoSizes, err := h.resolvePhoto(r, params)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, err.Error())
		return
	}
	if photoRef == "" {
		writeError(w, http.StatusBadRequest, 400, "photo is required")
		return
	}
	caption, _ := params.String("caption", "")
	replyToMessageID, err := params.ReplyToMessageID()
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "reply_to_message_id must be an integer")
		return
	}
	replyMarkup, err := params.RawJSON("reply_markup")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "reply_markup must be valid JSON")
		return
	}
	captionEntities, err := params.Entities("caption_entities")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "caption_entities must be valid JSON")
		return
	}
	captionParseMode, _ := params.String("parse_mode", "")

	msg, err := h.store.SaveBotMessage(r.Context(), store.BotMessageInput{
		From:             h.bot,
		ChatID:           chatID,
		Caption:          caption,
		CaptionEntities:  captionEntities,
		CaptionParseMode: captionParseMode,
		Photo:            photoSizes,
		PhotoURL:         photoRef,
		MediaKind:        "photo",
		MediaURL:         photoRef,
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

func (h *Handler) resolvePhoto(r *http.Request, params parameters) (string, []tg.PhotoSize, error) {
	if upload, ok := params.Upload("photo"); ok {
		file, err := h.saveUploadedMedia(r, "photo", upload)
		if err != nil {
			return "", nil, err
		}
		return simFileURL(file), photoSizesForFile(file), nil
	}

	photo, _ := params.String("photo", "")
	if photo == "" {
		return "", nil, nil
	}
	if file, ok := h.lookupMedia(r, photo); ok {
		return simFileURL(file), photoSizesForFile(file), nil
	}
	return photo, nil, nil
}

type resolvedMedia struct {
	Kind      string
	URL       string
	Document  *tg.FileRef
	Audio     *tg.FileRef
	Video     *tg.FileRef
	Animation *tg.FileRef
	Voice     *tg.FileRef
	VideoNote *tg.FileRef
	Sticker   *tg.StickerRef
}

func (h *Handler) resolveGenericMedia(r *http.Request, method string, params parameters, fallbackKind, fallbackURL string) (resolvedMedia, error) {
	out := resolvedMedia{Kind: fallbackKind, URL: fallbackURL}
	field, kind, ok := genericMediaField(method)
	if !ok {
		return out, nil
	}
	out.Kind = kind

	if upload, ok := params.Upload(field); ok {
		file, err := h.saveUploadedMedia(r, kind, upload)
		if err != nil {
			return resolvedMedia{}, err
		}
		out.URL = simFileURL(file)
		out.assignFile(kind, fileRefForMediaFile(file))
		return out, nil
	}

	raw, _ := params.String(field, "")
	if raw == "" {
		return out, nil
	}
	if file, ok := h.lookupMedia(r, raw); ok {
		out.URL = simFileURL(file)
		out.assignFile(kind, fileRefForMediaFile(file))
		return out, nil
	}
	out.URL = raw
	out.assignFile(kind, fileRefForExternal(kind, raw))
	return out, nil
}

func (m *resolvedMedia) assignFile(kind string, ref tg.FileRef) {
	switch kind {
	case "document":
		m.Document = &ref
	case "audio":
		m.Audio = &ref
	case "video":
		m.Video = &ref
	case "animation":
		m.Animation = &ref
	case "voice":
		m.Voice = &ref
	case "video_note":
		m.VideoNote = &ref
	case "sticker":
		m.Sticker = stickerForFile(ref)
	}
}

func (h *Handler) handleSendRichMessage(w http.ResponseWriter, r *http.Request, params parameters) {
	chatID, err := params.ChatIDValue("chat_id", 0)
	if err != nil || chatID == 0 {
		writeError(w, http.StatusBadRequest, 400, "chat_id is required")
		return
	}
	richMessage, err := params.RawJSON("rich_message")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "rich_message must be valid JSON")
		return
	}
	if len(richMessage) == 0 {
		writeError(w, http.StatusBadRequest, 400, "rich_message is required")
		return
	}
	replyMarkup, err := params.RawJSON("reply_markup")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "reply_markup must be valid JSON")
		return
	}

	msg, err := h.store.SaveBotMessage(r.Context(), store.BotMessageInput{
		From:        h.bot,
		ChatID:      chatID,
		Text:        params.StringValue("text", ""),
		RichMessage: richMessage,
		ReplyMarkup: replyMarkup,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, err.Error())
		return
	}
	h.broadcastMessage("created", msg)
	writeOK(w, msg)
}

func (h *Handler) handleSendDraft(w http.ResponseWriter, params parameters, method string) {
	chatID, err := params.ChatIDValue("chat_id", 0)
	if err != nil || chatID == 0 {
		writeError(w, http.StatusBadRequest, 400, "chat_id is required")
		return
	}
	draftID, err := params.Int64("draft_id", 0)
	if err != nil || draftID == 0 {
		writeError(w, http.StatusBadRequest, 400, "draft_id is required")
		return
	}
	entities, err := params.Entities("entities")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "entities must be valid JSON")
		return
	}
	richMessage, err := params.RawJSON("rich_message")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "rich_message must be valid JSON")
		return
	}
	parseMode, _ := params.String("parse_mode", "")
	text, _ := params.String("text", "")
	if text == "" && method == "sendMessageDraft" && len(richMessage) == 0 {
		text = "Thinking..."
	}
	if h.hub != nil {
		h.hub.Broadcast("message_draft", map[string]any{
			"chat_id":      chatID,
			"draft_id":     draftID,
			"text":         text,
			"entities":     entities,
			"parse_mode":   parseMode,
			"rich_message": optionalRawJSON(richMessage),
			"until":        time.Now().Add(5 * time.Second).UnixMilli(),
		})
	}
	writeOK(w, true)
}

func (h *Handler) handleGenericSendMessage(w http.ResponseWriter, r *http.Request, method string, params parameters) {
	chatID, err := params.ChatIDValue("chat_id", 0)
	if err != nil || chatID == 0 {
		writeError(w, http.StatusBadRequest, 400, "chat_id is required")
		return
	}
	replyMarkup, err := params.RawJSON("reply_markup")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "reply_markup must be valid JSON")
		return
	}
	entities, err := params.Entities("entities")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "entities must be valid JSON")
		return
	}
	captionEntities, err := params.Entities("caption_entities")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "caption_entities must be valid JSON")
		return
	}
	text, caption, mediaKind, mediaURL := genericMessageContent(method, params)
	resolvedMedia, err := h.resolveGenericMedia(r, method, params, mediaKind, mediaURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, err.Error())
		return
	}
	parseMode, _ := params.String("parse_mode", "")

	msg, err := h.store.SaveBotMessage(r.Context(), store.BotMessageInput{
		From:             h.bot,
		ChatID:           chatID,
		Text:             text,
		Entities:         entities,
		ParseMode:        parseMode,
		Caption:          caption,
		CaptionEntities:  captionEntities,
		CaptionParseMode: parseMode,
		Document:         resolvedMedia.Document,
		Audio:            resolvedMedia.Audio,
		Video:            resolvedMedia.Video,
		Animation:        resolvedMedia.Animation,
		Voice:            resolvedMedia.Voice,
		VideoNote:        resolvedMedia.VideoNote,
		Sticker:          resolvedMedia.Sticker,
		MediaKind:        resolvedMedia.Kind,
		MediaURL:         resolvedMedia.URL,
		ReplyMarkup:      replyMarkup,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, err.Error())
		return
	}
	h.broadcastMessage("created", msg)
	writeOK(w, msg)
}

func (h *Handler) handleSendMediaGroup(w http.ResponseWriter, r *http.Request, params parameters) {
	chatID, err := params.ChatIDValue("chat_id", 0)
	if err != nil || chatID == 0 {
		writeError(w, http.StatusBadRequest, 400, "chat_id is required")
		return
	}
	mediaRaw, err := params.RawJSON("media")
	if err != nil || len(mediaRaw) == 0 {
		writeError(w, http.StatusBadRequest, 400, "media must be valid JSON")
		return
	}
	var media []map[string]any
	if err := json.Unmarshal(mediaRaw, &media); err != nil {
		writeError(w, http.StatusBadRequest, 400, "media must be a JSON array")
		return
	}
	out := make([]tg.Message, 0, len(media))
	for index, item := range media {
		kind := stringFromAny(item["type"], "media")
		urlValue := stringFromAny(item["media"], "")
		caption := stringFromAny(item["caption"], "")
		if caption == "" {
			caption = fmt.Sprintf("%s #%d", methodHumanLabel(kind), index+1)
		}
		msg, err := h.store.SaveBotMessage(r.Context(), store.BotMessageInput{
			From:      h.bot,
			ChatID:    chatID,
			Caption:   caption,
			MediaKind: kind,
			MediaURL:  urlValue,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, 500, err.Error())
			return
		}
		h.broadcastMessage("created", msg)
		out = append(out, msg)
	}
	writeOK(w, out)
}

func (h *Handler) handleCopyMessage(w http.ResponseWriter, r *http.Request, params parameters) {
	chatID, err := params.ChatIDValue("chat_id", 0)
	if err != nil || chatID == 0 {
		writeError(w, http.StatusBadRequest, 400, "chat_id is required")
		return
	}
	caption, _ := params.String("caption", "")
	if caption == "" {
		caption = "Copied message"
	}
	msg, err := h.store.SaveBotMessage(r.Context(), store.BotMessageInput{
		From:   h.bot,
		ChatID: chatID,
		Text:   caption,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, 500, err.Error())
		return
	}
	h.broadcastMessage("created", msg)
	writeOK(w, map[string]any{"message_id": msg.MessageID})
}

func (h *Handler) handleMessageIDList(w http.ResponseWriter, method string, params parameters) {
	ids, err := params.Int64Slice("message_ids")
	if err != nil || len(ids) == 0 {
		id, err := params.Int64("message_id", 0)
		if err != nil || id == 0 {
			writeError(w, http.StatusBadRequest, 400, "message_id or message_ids is required")
			return
		}
		ids = []int64{id}
	}
	result := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		result = append(result, map[string]any{"message_id": id})
	}
	if method == "forwardMessages" {
		writeOK(w, result)
		return
	}
	writeOK(w, result)
}

func (h *Handler) handleDeleteMessages(w http.ResponseWriter, r *http.Request, params parameters) {
	chatID, err := params.ChatIDValue("chat_id", 0)
	if err != nil || chatID == 0 {
		writeError(w, http.StatusBadRequest, 400, "chat_id is required")
		return
	}
	ids, err := params.Int64Slice("message_ids")
	if err != nil || len(ids) == 0 {
		writeError(w, http.StatusBadRequest, 400, "message_ids is required")
		return
	}
	for _, id := range ids {
		msg, err := h.store.DeleteMessage(r.Context(), chatID, id)
		if err == nil {
			h.broadcastMessage("deleted", msg)
		}
	}
	writeOK(w, true)
}

func (h *Handler) handleGetCustomEmojiStickers(w http.ResponseWriter, params parameters) {
	ids, err := params.StringSlice("custom_emoji_ids")
	if err != nil || len(ids) == 0 {
		writeError(w, http.StatusBadRequest, 400, "custom_emoji_ids is required")
		return
	}
	stickers := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		stickers = append(stickers, map[string]any{
			"file_id":         "custom_emoji_" + id,
			"file_unique_id":  "custom_emoji_" + id + "_unique",
			"type":            "custom_emoji",
			"width":           100,
			"height":          100,
			"is_animated":     false,
			"is_video":        false,
			"emoji":           "✨",
			"custom_emoji_id": id,
		})
	}
	writeOK(w, stickers)
}

func (h *Handler) handleGetFile(w http.ResponseWriter, r *http.Request, params parameters) {
	fileID, _ := params.String("file_id", "")
	if fileID == "" {
		writeError(w, http.StatusBadRequest, 400, "file_id is required")
		return
	}
	if file, ok := h.lookupMedia(r, fileID); ok {
		writeOK(w, fileResult(file))
		return
	}
	if h.cfg.EffectiveAPIMode() == config.APIModeStrict {
		writeError(w, http.StatusBadRequest, 400, "file not found")
		return
	}
	writeOK(w, map[string]any{
		"file_id":        fileID,
		"file_unique_id": fileID + "_unique",
		"file_size":      0,
		"file_path":      "sim/" + fileID,
	})
}

func validChatAction(action string) bool {
	switch action {
	case "typing",
		"upload_photo",
		"record_video",
		"upload_video",
		"record_voice",
		"upload_voice",
		"upload_document",
		"choose_sticker",
		"find_location",
		"record_video_note",
		"upload_video_note":
		return true
	default:
		return false
	}
}

// handledInlineEdit answers edit*/stop* calls that target an inline message via
// inline_message_id. Real Telegram returns True (not a Message) for those, since
// the bot does not own a chat message. The simulator does not store inline
// messages, so it acknowledges without mutating state.
func (h *Handler) handledInlineEdit(w http.ResponseWriter, params parameters) bool {
	inlineID, _ := params.String("inline_message_id", "")
	if inlineID == "" {
		return false
	}
	writeOK(w, true)
	return true
}

func (h *Handler) handleEditMessageText(w http.ResponseWriter, r *http.Request, params parameters) {
	if h.handledInlineEdit(w, params) {
		return
	}
	chatID, err := params.ChatIDValue("chat_id", 0)
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
	richMessage, err := params.RawJSON("rich_message")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "rich_message must be valid JSON")
		return
	}
	if text == "" && len(richMessage) == 0 {
		writeError(w, http.StatusBadRequest, 400, "text or rich_message is required")
		return
	}
	replyMarkup, err := params.RawJSON("reply_markup")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "reply_markup must be valid JSON")
		return
	}
	entities, err := params.Entities("entities")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "entities must be valid JSON")
		return
	}
	parseMode, _ := params.String("parse_mode", "")

	msg, err := h.store.EditMessageText(r.Context(), store.EditMessageTextInput{
		ChatID:      chatID,
		MessageID:   messageID,
		Text:        text,
		Entities:    entities,
		ParseMode:   parseMode,
		RichMessage: richMessage,
		ReplyMarkup: replyMarkup,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	h.broadcastMessage("edited", msg)
	writeOK(w, msg)
}

func (h *Handler) handleEditMessageCaption(w http.ResponseWriter, r *http.Request, params parameters) {
	if h.handledInlineEdit(w, params) {
		return
	}
	chatID, err := params.ChatIDValue("chat_id", 0)
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
	captionEntities, err := params.Entities("caption_entities")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "caption_entities must be valid JSON")
		return
	}
	caption, _ := params.String("caption", "")
	parseMode, _ := params.String("parse_mode", "")

	msg, err := h.store.EditMessageCaption(r.Context(), store.EditMessageCaptionInput{
		ChatID:           chatID,
		MessageID:        messageID,
		Caption:          caption,
		CaptionEntities:  captionEntities,
		CaptionParseMode: parseMode,
		ReplyMarkup:      replyMarkup,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	h.broadcastMessage("edited", msg)
	writeOK(w, msg)
}

func (h *Handler) handleEditMessageMedia(w http.ResponseWriter, r *http.Request, params parameters) {
	if h.handledInlineEdit(w, params) {
		return
	}
	chatID, err := params.ChatIDValue("chat_id", 0)
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
	mediaRaw, err := params.RawJSON("media")
	if err != nil {
		writeError(w, http.StatusBadRequest, 400, "media must be valid JSON")
		return
	}
	input := store.EditMessageMediaInput{ChatID: chatID, MessageID: messageID, ReplyMarkup: replyMarkup}
	if len(mediaRaw) > 0 {
		var media map[string]any
		if err := json.Unmarshal(mediaRaw, &media); err != nil {
			writeError(w, http.StatusBadRequest, 400, "media must be a JSON object")
			return
		}
		input.MediaKind = stringFromAny(media["type"], "")
		input.MediaURL = stringFromAny(media["media"], "")
		input.Caption = stringFromAny(media["caption"], "")
	}

	msg, err := h.store.EditMessageMedia(r.Context(), input)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	h.broadcastMessage("edited", msg)
	writeOK(w, msg)
}

func (h *Handler) handleEditMessageReplyMarkup(w http.ResponseWriter, r *http.Request, params parameters) {
	if h.handledInlineEdit(w, params) {
		return
	}
	chatID, err := params.ChatIDValue("chat_id", 0)
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
	chatID, err := params.ChatIDValue("chat_id", 0)
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

func (h *Handler) handleKnownMethodStub(w http.ResponseWriter, method string, params parameters) {
	h.logger.Debug("stubbed bot api method", "method", method)
	switch method {
	case "logOut", "close",
		"banChatMember", "unbanChatMember", "restrictChatMember", "promoteChatMember",
		"setChatAdministratorCustomTitle", "setChatMemberTag", "banChatSenderChat", "unbanChatSenderChat",
		"setChatPermissions", "approveChatJoinRequest", "declineChatJoinRequest", "answerChatJoinRequestQuery",
		"setChatPhoto", "deleteChatPhoto", "setChatTitle", "setChatDescription", "pinChatMessage", "unpinChatMessage",
		"unpinAllChatMessages", "leaveChat", "setChatStickerSet", "deleteChatStickerSet", "createForumTopic",
		"editForumTopic", "closeForumTopic", "reopenForumTopic", "deleteForumTopic", "unpinAllForumTopicMessages",
		"editGeneralForumTopic", "closeGeneralForumTopic", "reopenGeneralForumTopic", "hideGeneralForumTopic",
		"unhideGeneralForumTopic", "unpinAllGeneralForumTopicMessages", "answerGuestQuery",
		"setManagedBotAccessSettings", "setMyCommands", "deleteMyCommands",
		"setMyName", "setMyDescription", "setMyShortDescription", "setMyProfilePhoto", "removeMyProfilePhoto",
		"setChatMenuButton", "setMyDefaultAdministratorRights", "sendGift", "giftPremiumSubscription",
		"verifyUser", "verifyChat", "removeUserVerification", "removeChatVerification", "readBusinessMessage",
		"deleteBusinessMessages", "setBusinessAccountName", "setBusinessAccountUsername", "setBusinessAccountBio",
		"setBusinessAccountProfilePhoto", "removeBusinessAccountProfilePhoto", "setBusinessAccountGiftSettings",
		"transferBusinessAccountStars", "convertGiftToStars", "upgradeGift", "transferGift", "postStory",
		"repostStory", "editStory", "deleteStory", "answerInlineQuery", "answerShippingQuery",
		"answerPreCheckoutQuery", "refundStarPayment", "editUserStarSubscription", "setPassportDataErrors",
		"setGameScore", "deleteMessageReaction", "deleteAllMessageReactions", "createNewStickerSet",
		"addStickerToSet", "setStickerPositionInSet", "deleteStickerFromSet", "replaceStickerInSet",
		"setStickerEmojiList", "setStickerKeywords", "setStickerMaskPosition", "setStickerSetTitle",
		"setStickerSetThumbnail", "setCustomEmojiStickerSetThumbnail", "deleteStickerSet",
		"approveSuggestedPost", "declineSuggestedPost", "setMessageReaction", "setUserEmojiStatus":
		writeOK(w, true)
	case "uploadStickerFile":
		fileID, _ := params.String("sticker", "sim_file")
		writeOK(w, map[string]any{
			"file_id":        fileID,
			"file_unique_id": fileID + "_unique",
			"file_size":      0,
			"file_path":      "sim/" + fileID,
		})
	case "exportChatInviteLink", "createInvoiceLink", "getManagedBotToken", "replaceManagedBotToken":
		writeOK(w, "https://t.me/local_telegram_client_sim")
	case "createChatInviteLink", "editChatInviteLink", "createChatSubscriptionInviteLink", "editChatSubscriptionInviteLink", "revokeChatInviteLink":
		writeOK(w, map[string]any{
			"invite_link":          "https://t.me/+local-sim",
			"creator":              h.bot,
			"creates_join_request": false,
			"is_primary":           false,
			"is_revoked":           false,
		})
	case "getChat":
		chatID, _ := params.ChatIDValue("chat_id", 1)
		writeOK(w, tg.Chat{ID: chatID, Type: "private", FirstName: "Sim Chat"})
	case "getChatAdministrators":
		writeOK(w, []any{})
	case "getChatMemberCount":
		writeOK(w, 1)
	case "getChatMember":
		writeOK(w, map[string]any{"status": "member", "user": h.bot})
	case "getUserProfilePhotos":
		writeOK(w, map[string]any{"total_count": 0, "photos": []any{}})
	case "getUserProfileAudios":
		writeOK(w, map[string]any{"total_count": 0, "audios": []any{}})
	case "getForumTopicIconStickers", "getBusinessAccountGifts", "getUserGifts", "getChatGifts":
		writeOK(w, []any{})
	case "getUserPersonalChatMessages":
		writeOK(w, []any{})
	case "getUserChatBoosts":
		writeOK(w, map[string]any{"boosts": []any{}})
	case "getBusinessConnection":
		id, _ := params.String("business_connection_id", "sim_business_connection")
		writeOK(w, map[string]any{"id": id, "user": h.bot, "user_chat_id": h.bot.ID, "date": time.Now().Unix(), "is_enabled": true})
	case "getManagedBotAccessSettings":
		writeOK(w, map[string]any{"is_managed": false})
	case "getMyCommands":
		writeOK(w, []any{})
	case "getMyName":
		writeOK(w, map[string]any{"name": h.bot.FirstName})
	case "getMyDescription", "getMyShortDescription":
		writeOK(w, map[string]any{"description": ""})
	case "getChatMenuButton":
		writeOK(w, map[string]any{"type": "default"})
	case "getMyDefaultAdministratorRights":
		writeOK(w, map[string]any{})
	case "getAvailableGifts":
		writeOK(w, map[string]any{"gifts": []any{}})
	case "getBusinessAccountStarBalance", "getMyStarBalance":
		writeOK(w, map[string]any{"amount": 0})
	case "getStarTransactions":
		writeOK(w, map[string]any{"transactions": []any{}})
	case "answerWebAppQuery":
		writeOK(w, map[string]any{"inline_message_id": "sim_inline_message"})
	case "savePreparedInlineMessage":
		writeOK(w, map[string]any{"id": "sim_prepared_inline_message", "expiration_date": time.Now().Add(time.Hour).Unix()})
	case "savePreparedKeyboardButton":
		writeOK(w, map[string]any{"id": "sim_prepared_keyboard_button", "expiration_date": time.Now().Add(time.Hour).Unix()})
	case "editMessageLiveLocation", "stopMessageLiveLocation", "editMessageChecklist", "stopPoll":
		writeOK(w, true)
	case "getStickerSet":
		name, _ := params.String("name", "sim_sticker_set")
		writeOK(w, map[string]any{"name": name, "title": name, "sticker_type": "regular", "stickers": []any{}})
	case "getGameHighScores":
		writeOK(w, []any{})
	case "sendChatJoinRequestWebApp":
		writeOK(w, map[string]any{"sent": true})
	default:
		writeOK(w, true)
	}
}

func (h *Handler) saveUploadedMedia(r *http.Request, kind string, upload uploadedFile) (media.File, error) {
	if h.media == nil {
		return media.File{}, errors.New("media store is unavailable")
	}
	return h.media.Save(r.Context(), media.FileInput{
		Kind:        kind,
		FieldName:   upload.FieldName,
		FileName:    upload.FileName,
		ContentType: upload.ContentType,
		Data:        upload.Data,
	})
}

func (h *Handler) lookupMedia(r *http.Request, fileID string) (media.File, bool) {
	if h.media == nil || fileID == "" {
		return media.File{}, false
	}
	file, err := h.media.Get(r.Context(), fileID)
	return file, err == nil
}

func fileResult(file media.File) map[string]any {
	// file_path is consumed by SDKs as <base>/file/bot<token>/<file_path>, which
	// the server serves from the media store; keep it equal to the file id.
	return map[string]any{
		"file_id":        file.ID,
		"file_unique_id": file.UniqueID,
		"file_size":      file.Size,
		"file_path":      file.ID,
	}
}

func fileRefForMediaFile(file media.File) tg.FileRef {
	return tg.FileRef{
		FileID:       file.ID,
		FileUniqueID: file.UniqueID,
		FileName:     file.FileName,
		MimeType:     file.ContentType,
		FileSize:     int(file.Size),
		Width:        640,
		Height:       480,
	}
}

func fileRefForExternal(kind, value string) tg.FileRef {
	id := deterministicMediaID(kind, value)
	return tg.FileRef{
		FileID:       id,
		FileUniqueID: id + "_unique",
		FileName:     externalFileName(value, kind),
		Width:        640,
		Height:       480,
	}
}

func stickerForFile(ref tg.FileRef) *tg.StickerRef {
	return &tg.StickerRef{
		FileID:       ref.FileID,
		FileUniqueID: ref.FileUniqueID,
		Type:         "regular",
		Width:        512,
		Height:       512,
		IsAnimated:   false,
		IsVideo:      false,
		FileSize:     ref.FileSize,
	}
}

func deterministicMediaID(kind, value string) string {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(kind))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(value))
	return fmt.Sprintf("%s_%08x", strings.ReplaceAll(kind, "_", ""), hash.Sum32())
}

func externalFileName(value, fallback string) string {
	cleaned := strings.TrimRight(value, "/")
	idx := strings.LastIndexByte(cleaned, '/')
	if idx >= 0 && idx < len(cleaned)-1 {
		return cleaned[idx+1:]
	}
	return fallback
}

func genericMediaField(method string) (field, kind string, ok bool) {
	switch method {
	case "sendDocument":
		return "document", "document", true
	case "sendVideo":
		return "video", "video", true
	case "sendAnimation":
		return "animation", "animation", true
	case "sendAudio":
		return "audio", "audio", true
	case "sendVoice":
		return "voice", "voice", true
	case "sendVideoNote":
		return "video_note", "video_note", true
	case "sendSticker":
		return "sticker", "sticker", true
	case "sendLivePhoto":
		return "live_photo", "live_photo", true
	default:
		return "", "", false
	}
}

func simFileURL(file media.File) string {
	return "/_sim/file/" + url.PathEscape(file.ID)
}

func photoSizesForFile(file media.File) []tg.PhotoSize {
	return []tg.PhotoSize{
		{
			FileID:       file.ID,
			FileUniqueID: file.UniqueID,
			Width:        640,
			Height:       480,
			FileSize:     int(file.Size),
		},
	}
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

const (
	maxUploadBytes = 32 << 20 // 32 MiB cap on a single multipart file
	maxJSONBody    = 4 << 20  // 4 MiB cap on JSON/form bot API bodies
)

type uploadedFile struct {
	FieldName   string
	FileName    string
	ContentType string
	Size        int64
	Data        []byte
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
		decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxJSONBody))
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
		r.Body = http.MaxBytesReader(nil, r.Body, maxJSONBody)
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
		for key, files := range r.MultipartForm.File {
			if len(files) == 0 {
				continue
			}
			upload, err := readUploadedFile(key, files[0])
			if err != nil {
				return nil, err
			}
			params[key] = upload
		}
	default:
		return nil, fmt.Errorf("unsupported content type %q", contentType)
	}

	return params, nil
}

func readUploadedFile(fieldName string, header *multipart.FileHeader) (uploadedFile, error) {
	file, err := header.Open()
	if err != nil {
		return uploadedFile{}, fmt.Errorf("open uploaded %s: %w", fieldName, err)
	}
	defer file.Close()

	limited := io.LimitReader(file, maxUploadBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return uploadedFile{}, fmt.Errorf("read uploaded %s: %w", fieldName, err)
	}
	if len(data) > maxUploadBytes {
		return uploadedFile{}, fmt.Errorf("%s upload exceeds 32MiB", fieldName)
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(data)
	}
	return uploadedFile{
		FieldName:   fieldName,
		FileName:    header.Filename,
		ContentType: contentType,
		Size:        int64(len(data)),
		Data:        data,
	}, nil
}

func genericMessageContent(method string, params parameters) (text, caption, mediaKind, mediaURL string) {
	text, _ = params.String("text", "")
	caption, _ = params.String("caption", "")
	mediaKind = strings.TrimPrefix(method, "send")
	if mediaKind == method || mediaKind == "" {
		mediaKind = method
	}
	mediaKind = strings.ToLower(mediaKind[:1]) + mediaKind[1:]
	for _, key := range []string{"photo", "live_photo", "audio", "document", "video", "animation", "voice", "video_note", "sticker", "media"} {
		if value, ok := params.String(key, ""); ok && value != "" {
			mediaURL = value
			break
		}
	}
	if text == "" && caption == "" {
		switch method {
		case "sendDice":
			text = "🎲 Dice"
		case "sendLocation":
			text = fmt.Sprintf("Location: %s, %s", params.StringValue("latitude", "?"), params.StringValue("longitude", "?"))
		case "sendVenue":
			text = "Venue: " + params.StringValue("title", "Untitled venue")
		case "sendContact":
			text = "Contact: " + strings.TrimSpace(params.StringValue("first_name", "")+" "+params.StringValue("last_name", ""))
		case "sendPoll":
			text = "Poll: " + params.StringValue("question", "Untitled poll")
		case "sendChecklist":
			text = "Checklist"
		case "sendGame":
			text = "Game: " + params.StringValue("game_short_name", "game")
		case "forwardMessage":
			text = "Forwarded message"
		default:
			caption = methodHumanLabel(mediaKind)
		}
	}
	return text, caption, mediaKind, mediaURL
}

func methodHumanLabel(kind string) string {
	if kind == "" {
		return "Bot API message"
	}
	return "Bot API " + kind
}

func optionalRawJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

func stringFromAny(value any, fallback string) string {
	if value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return fallback
		}
		return typed
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
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
	case uploadedFile:
		return typed.FileName, true
	default:
		return fmt.Sprint(typed), true
	}
}

func (p parameters) StringValue(key, fallback string) string {
	value, _ := p.String(key, fallback)
	return value
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
	chatID, err := p.ChatIDValue("chat_id", 0)
	if err != nil || chatID == 0 {
		return nil
	}
	return &chatID
}

func (p parameters) ChatIDValue(key string, fallback int64) (int64, error) {
	value, ok := p[key]
	if !ok || value == nil {
		return fallback, nil
	}
	if raw, ok := value.(string); ok {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return fallback, nil
		}
		if strings.HasPrefix(raw, "@") {
			return syntheticChatID(raw), nil
		}
		return strconv.ParseInt(raw, 10, 64)
	}
	return p.Int64(key, fallback)
}

func (p parameters) TraceParams() map[string]any {
	out := make(map[string]any, len(p))
	for key, value := range p {
		if upload, ok := value.(uploadedFile); ok {
			out[key] = map[string]any{
				"uploaded":     true,
				"filename":     upload.FileName,
				"content_type": upload.ContentType,
				"size":         upload.Size,
			}
			continue
		}
		out[key] = value
	}
	return out
}

func (p parameters) Upload(key string) (uploadedFile, bool) {
	value, ok := p[key]
	if !ok || value == nil {
		return uploadedFile{}, false
	}
	upload, ok := value.(uploadedFile)
	return upload, ok
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

// ReplyToMessageID returns the reply target from either the legacy
// reply_to_message_id field or the modern reply_parameters object.
func (p parameters) ReplyToMessageID() (int64, error) {
	id, err := p.Int64("reply_to_message_id", 0)
	if err != nil {
		return 0, err
	}
	if id != 0 {
		return id, nil
	}
	raw, err := p.RawJSON("reply_parameters")
	if err != nil || len(raw) == 0 {
		return 0, err
	}
	var rp struct {
		MessageID int64 `json:"message_id"`
	}
	if err := json.Unmarshal(raw, &rp); err != nil {
		return 0, nil
	}
	return rp.MessageID, nil
}

func (p parameters) Entities(key string) ([]tg.MessageEntity, error) {
	raw, err := p.RawJSON(key)
	if err != nil || len(raw) == 0 {
		return nil, err
	}
	var entities []tg.MessageEntity
	if err := json.Unmarshal(raw, &entities); err != nil {
		return nil, err
	}
	return entities, nil
}

func (p parameters) StringSlice(key string) ([]string, error) {
	value, ok := p[key]
	if !ok || value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case []string:
		return typed, nil
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, stringFromAny(item, ""))
		}
		return out, nil
	case string:
		if typed == "" {
			return nil, nil
		}
		var out []string
		if json.Valid([]byte(typed)) {
			if err := json.Unmarshal([]byte(typed), &out); err == nil {
				return out, nil
			}
		}
		return []string{typed}, nil
	default:
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
}

func (p parameters) Int64Slice(key string) ([]int64, error) {
	value, ok := p[key]
	if !ok || value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case []int64:
		return typed, nil
	case []any:
		out := make([]int64, 0, len(typed))
		for _, item := range typed {
			switch value := item.(type) {
			case json.Number:
				id, err := value.Int64()
				if err != nil {
					return nil, err
				}
				out = append(out, id)
			case float64:
				out = append(out, int64(value))
			case string:
				id, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return nil, err
				}
				out = append(out, id)
			default:
				return nil, fmt.Errorf("%s must be an array of integers", key)
			}
		}
		return out, nil
	case string:
		if typed == "" {
			return nil, nil
		}
		var out []int64
		if err := json.Unmarshal([]byte(typed), &out); err != nil {
			return nil, err
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s must be an array of integers", key)
	}
}

func syntheticChatID(value string) int64 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(strings.ToLower(value)))
	return int64(2_000_000_000 + hash.Sum32()%1_000_000_000)
}
