package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mymmrac/telego"

	"github.com/Zulut30/local-telegram-client/internal/config"
	"github.com/Zulut30/local-telegram-client/internal/store"
	"github.com/Zulut30/local-telegram-client/internal/tg"
)

func TestBotAPILongPollFlowWithTelego(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{Mode: config.ModeLocal, BotToken: "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc", BufferSize: 100}
	handler := NewWithStore(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	bot, err := telego.NewBot(cfg.BotToken, telego.WithAPIServer(srv.URL), telego.WithDiscardLogger())
	if err != nil {
		t.Fatalf("NewBot returned error: %v", err)
	}

	me, err := bot.GetMe()
	if err != nil {
		t.Fatalf("GetMe returned error: %v", err)
	}
	if !me.IsBot || me.Username == "" {
		t.Fatalf("GetMe user = %#v, want bot identity with username", me)
	}

	injectBody := bytes.NewBufferString(`{"type":"message","chat_id":42,"user_id":7,"username":"dev","text":"/start"}`)
	resp, err := http.Post(srv.URL+"/_sim/inject", "application/json", injectBody)
	if err != nil {
		t.Fatalf("inject request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("inject status = %d, body = %s", resp.StatusCode, body)
	}

	updates, err := bot.GetUpdates(&telego.GetUpdatesParams{Limit: 10, Timeout: 1})
	if err != nil {
		t.Fatalf("GetUpdates returned error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("updates length = %d, want 1", len(updates))
	}
	update := updates[0]
	if update.Message == nil {
		t.Fatal("update.Message is nil")
	}
	if update.Message.Text != "/start" {
		t.Fatalf("message text = %q, want /start", update.Message.Text)
	}
	if update.Message.Chat.ID != 42 {
		t.Fatalf("chat id = %d, want 42", update.Message.Chat.ID)
	}

	sent, err := bot.SendMessage(&telego.SendMessageParams{
		ChatID: telego.ChatID{ID: update.Message.Chat.ID},
		Text:   "hello from bot",
	})
	if err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}
	if sent.Text != "hello from bot" {
		t.Fatalf("sent text = %q, want bot response", sent.Text)
	}

	acked, err := bot.GetUpdates(&telego.GetUpdatesParams{Offset: update.UpdateID + 1})
	if err != nil {
		t.Fatalf("ack GetUpdates returned error: %v", err)
	}
	if len(acked) != 0 {
		t.Fatalf("acked updates length = %d, want 0", len(acked))
	}

	state, err := st.State(context.Background())
	if err != nil {
		t.Fatalf("State returned error: %v", err)
	}
	messages := state.Messages["42"]
	if len(messages) != 2 {
		encoded, _ := json.Marshal(messages)
		t.Fatalf("stored messages length = %d, want 2: %s", len(messages), encoded)
	}
	if messages[0].Text != "/start" || messages[1].Text != "hello from bot" {
		t.Fatalf("stored message texts = %q, %q", messages[0].Text, messages[1].Text)
	}
	if messages[1].From == nil || !messages[1].From.IsBot {
		t.Fatalf("bot response From = %#v, want bot user", messages[1].From)
	}
}

func TestBotAPIFidelityEditsRepliesAndFilters(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{Mode: config.ModeLocal, BotToken: "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc", BufferSize: 100}
	handler := NewWithStore(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// Base message to reply to and to caption-edit.
	baseEnv := botCall(t, srv.URL, cfg.BotToken, "sendMessage", map[string]any{
		"chat_id": 42,
		"text":    "question",
	}, http.StatusOK)
	base := decodeBotResult[tg.Message](t, baseEnv)

	// reply_to_message_id should be resolved into a nested reply_to_message.
	replyEnv := botCall(t, srv.URL, cfg.BotToken, "sendMessage", map[string]any{
		"chat_id":             42,
		"text":                "answer",
		"reply_to_message_id": base.MessageID,
		"message_thread_id":   99,
	}, http.StatusOK)
	reply := decodeBotResult[tg.Message](t, replyEnv)
	if reply.ReplyToMessage == nil || reply.ReplyToMessage.Text != "question" {
		t.Fatalf("reply_to_message not populated: %+v", reply.ReplyToMessage)
	}
	if reply.MessageThreadID != 99 {
		t.Fatalf("message_thread_id = %d, want 99", reply.MessageThreadID)
	}

	// editMessageCaption must return a full Message, not bare true.
	capEnv := botCall(t, srv.URL, cfg.BotToken, "editMessageCaption", map[string]any{
		"chat_id":    42,
		"message_id": base.MessageID,
		"caption":    "Updated caption",
	}, http.StatusOK)
	edited := decodeBotResult[tg.Message](t, capEnv)
	if edited.Caption != "Updated caption" {
		t.Fatalf("editMessageCaption caption = %q, want Updated caption", edited.Caption)
	}

	// Inline edits (inline_message_id) return true, since no chat message is owned.
	inlineEnv := botCall(t, srv.URL, cfg.BotToken, "editMessageText", map[string]any{
		"inline_message_id": "inline_abc",
		"text":              "x",
	}, http.StatusOK)
	var inlineOK bool
	if err := json.Unmarshal(inlineEnv.Result, &inlineOK); err != nil || !inlineOK {
		t.Fatalf("inline edit result = %s (err %v), want true", inlineEnv.Result, err)
	}

	// allowed_updates filters out message updates when only callback_query is requested.
	injectBody := bytes.NewBufferString(`{"type":"message","chat_id":7,"text":"hi"}`)
	resp, err := http.Post(srv.URL+"/_sim/inject", "application/json", injectBody)
	if err != nil {
		t.Fatalf("inject: %v", err)
	}
	resp.Body.Close()

	filteredEnv := botCall(t, srv.URL, cfg.BotToken, "getUpdates", map[string]any{
		"allowed_updates": []string{"callback_query"},
	}, http.StatusOK)
	filtered := decodeBotResult[[]tg.Update](t, filteredEnv)
	if len(filtered) != 0 {
		t.Fatalf("allowed_updates=callback_query should hide message updates, got %d", len(filtered))
	}

	allEnv := botCall(t, srv.URL, cfg.BotToken, "getUpdates", map[string]any{
		"allowed_updates": []string{"message"},
	}, http.StatusOK)
	all := decodeBotResult[[]tg.Update](t, allEnv)
	if len(all) != 1 {
		t.Fatalf("allowed_updates=message should return the message, got %d", len(all))
	}
}

func TestBotAPIMessageMutationsAndCallbackAnswerEvents(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{Mode: config.ModeLocal, BotToken: "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc", BufferSize: 100}
	handler := NewWithStore(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	events, stopSSE := startSSE(t, srv.URL)
	t.Cleanup(stopSSE)

	sendEnv := botCall(t, srv.URL, cfg.BotToken, "sendMessage", map[string]any{
		"chat_id": 42,
		"text":    "Pick one",
		"reply_markup": map[string]any{
			"inline_keyboard": [][]map[string]string{{
				{"text": "Run", "callback_data": "run"},
			}},
		},
	}, http.StatusOK)
	sent := decodeBotResult[tg.Message](t, sendEnv)

	created := waitEventPayload[messageEventPayload](t, events, "message", func(payload messageEventPayload) bool {
		return payload.Op == "created" && payload.Message.MessageID == sent.MessageID
	})
	if created.Message.Text != "Pick one" || !rawJSONContains(created.Message.ReplyMarkup, "run") {
		t.Fatalf("created message = %#v, want original text and inline keyboard", created.Message)
	}

	editEnv := botCall(t, srv.URL, cfg.BotToken, "editMessageText", map[string]any{
		"chat_id":    42,
		"message_id": sent.MessageID,
		"text":       "Edited pick",
		"reply_markup": map[string]any{
			"inline_keyboard": [][]map[string]string{{
				{"text": "Done", "callback_data": "done"},
			}},
		},
	}, http.StatusOK)
	editedResult := decodeBotResult[tg.Message](t, editEnv)
	if editedResult.Text != "Edited pick" || !rawJSONContains(editedResult.ReplyMarkup, "done") {
		t.Fatalf("editMessageText result = %#v, want edited text and updated markup", editedResult)
	}

	editedTextEvent := waitEventPayload[messageEventPayload](t, events, "message", func(payload messageEventPayload) bool {
		return payload.Op == "edited" && payload.Message.MessageID == sent.MessageID && payload.Message.Text == "Edited pick"
	})
	if !rawJSONContains(editedTextEvent.Message.ReplyMarkup, "Done") {
		t.Fatalf("edited text event reply_markup = %s, want Done button", editedTextEvent.Message.ReplyMarkup)
	}

	markupEnv := botCall(t, srv.URL, cfg.BotToken, "editMessageReplyMarkup", map[string]any{
		"chat_id":    42,
		"message_id": sent.MessageID,
		"reply_markup": map[string]any{
			"inline_keyboard": [][]map[string]string{{
				{"text": "Again", "callback_data": "again"},
			}},
		},
	}, http.StatusOK)
	markupResult := decodeBotResult[tg.Message](t, markupEnv)
	if markupResult.Text != "Edited pick" || !rawJSONContains(markupResult.ReplyMarkup, "again") {
		t.Fatalf("editMessageReplyMarkup result = %#v, want same text and replaced markup", markupResult)
	}

	_ = waitEventPayload[messageEventPayload](t, events, "message", func(payload messageEventPayload) bool {
		return payload.Op == "edited" &&
			payload.Message.MessageID == sent.MessageID &&
			payload.Message.Text == "Edited pick" &&
			rawJSONContains(payload.Message.ReplyMarkup, "Again")
	})

	deleteEnv := botCall(t, srv.URL, cfg.BotToken, "deleteMessage", map[string]any{
		"chat_id":    42,
		"message_id": sent.MessageID,
	}, http.StatusOK)
	deletedOK := decodeBotResult[bool](t, deleteEnv)
	if !deletedOK {
		t.Fatal("deleteMessage result = false, want true")
	}
	deletedEvent := waitEventPayload[messageEventPayload](t, events, "message", func(payload messageEventPayload) bool {
		return payload.Op == "deleted" && payload.Message.MessageID == sent.MessageID
	})
	if deletedEvent.Message.Text != "Edited pick" {
		t.Fatalf("deleted event text = %q, want last edited text", deletedEvent.Message.Text)
	}

	answerEnv := botCall(t, srv.URL, cfg.BotToken, "answerCallbackQuery", map[string]any{
		"callback_query_id": "cb_manual",
		"text":              "Saved",
		"show_alert":        true,
	}, http.StatusOK)
	if ok := decodeBotResult[bool](t, answerEnv); !ok {
		t.Fatal("answerCallbackQuery result = false, want true")
	}
	answerEvent := waitEventPayload[callbackAnswerEventPayload](t, events, "callback_answer", func(payload callbackAnswerEventPayload) bool {
		return payload.CallbackQueryID == "cb_manual"
	})
	if answerEvent.Text != "Saved" || !answerEvent.ShowAlert {
		t.Fatalf("callback answer event = %#v, want alert text", answerEvent)
	}

	state, err := st.State(context.Background())
	if err != nil {
		t.Fatalf("State returned error: %v", err)
	}
	if messages := state.Messages["42"]; len(messages) != 0 {
		encoded, _ := json.Marshal(messages)
		t.Fatalf("stored messages length = %d, want 0 after delete: %s", len(messages), encoded)
	}

	missingEnv := botCall(t, srv.URL, cfg.BotToken, "editMessageText", map[string]any{
		"chat_id":    42,
		"message_id": sent.MessageID,
		"text":       "Nope",
	}, http.StatusBadRequest)
	if missingEnv.OK || missingEnv.ErrorCode != 400 || missingEnv.Description != "message not found" {
		t.Fatalf("missing edit response = %#v, want message not found 400", missingEnv)
	}
}

func TestBotAPISendChatActionValidation(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{Mode: config.ModeLocal, BotToken: "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc", BufferSize: 100}
	handler := NewWithStore(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	validEnv := botCall(t, srv.URL, cfg.BotToken, "sendChatAction", map[string]any{
		"chat_id": 42,
		"action":  telego.ChatActionUploadPhoto,
	}, http.StatusOK)
	if ok := decodeBotResult[bool](t, validEnv); !ok {
		t.Fatal("sendChatAction result = false, want true")
	}

	invalidEnv := botCall(t, srv.URL, cfg.BotToken, "sendChatAction", map[string]any{
		"chat_id": 42,
		"action":  "cook_dinner",
	}, http.StatusBadRequest)
	if invalidEnv.OK || invalidEnv.ErrorCode != 400 || invalidEnv.Description != "action is invalid" {
		t.Fatalf("invalid sendChatAction response = %#v, want action validation error", invalidEnv)
	}
}

func TestBotAPIFormattingRichMessagesAndRegistry(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{Mode: config.ModeLocal, BotToken: "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc", BufferSize: 100}
	handler := NewWithStore(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	sendEnv := botCall(t, srv.URL, cfg.BotToken, "SENDMESSAGE", map[string]any{
		"chat_id":    42,
		"text":       "Premium ✨ bold",
		"parse_mode": "HTML",
		"entities": []map[string]any{
			{"type": "custom_emoji", "offset": 8, "length": 1, "custom_emoji_id": "emoji_42", "alternative_text": "✨"},
			{"type": "bold", "offset": 10, "length": 4},
		},
	}, http.StatusOK)
	sent := decodeBotResult[tg.Message](t, sendEnv)
	if sent.Text != "Premium ✨ bold" || sent.ParseMode != "HTML" {
		t.Fatalf("sent text/parse_mode = %q/%q", sent.Text, sent.ParseMode)
	}
	if len(sent.Entities) != 2 || sent.Entities[0].CustomEmojiID != "emoji_42" {
		t.Fatalf("sent entities = %#v, want custom emoji entity", sent.Entities)
	}

	stickersEnv := botCall(t, srv.URL, cfg.BotToken, "getCustomEmojiStickers", map[string]any{
		"custom_emoji_ids": []string{"emoji_42"},
	}, http.StatusOK)
	stickers := decodeBotResult[[]map[string]any](t, stickersEnv)
	if len(stickers) != 1 || stickers[0]["custom_emoji_id"] != "emoji_42" {
		t.Fatalf("custom emoji stickers = %#v", stickers)
	}

	richPayload := map[string]any{
		"html": "<p><b>Меню</b></p><table><tr><td>Блюдо</td><td>Цена</td></tr><tr><td>Паста</td><td>12</td></tr></table>",
	}
	richEnv := botCall(t, srv.URL, cfg.BotToken, "sendRichMessage", map[string]any{
		"chat_id":      42,
		"rich_message": richPayload,
	}, http.StatusOK)
	richMessage := decodeBotResult[tg.Message](t, richEnv)
	if len(richMessage.RichMessage) == 0 || !rawJSONContains(richMessage.RichMessage, "table") {
		t.Fatalf("rich_message = %s, want table payload", richMessage.RichMessage)
	}

	videoEnv := botCall(t, srv.URL, cfg.BotToken, "sendVideo", map[string]any{
		"chat_id": 42,
		"video":   "https://example.test/video.mp4",
		"caption": "Видео",
	}, http.StatusOK)
	video := decodeBotResult[tg.Message](t, videoEnv)
	if video.MediaKind != "video" || video.MediaURL != "https://example.test/video.mp4" {
		t.Fatalf("video message = %#v, want generic video message", video)
	}
	if video.Video == nil || video.Video.FileID == "" || video.Video.FileName != "video.mp4" {
		t.Fatalf("video field = %#v, want Telegram-like video metadata", video.Video)
	}

	unknownEnv := botCall(t, srv.URL, cfg.BotToken, "definitelyNotTelegram", map[string]any{}, http.StatusNotFound)
	if unknownEnv.OK || unknownEnv.Description != "method not found" {
		t.Fatalf("unknown method response = %#v, want method not found", unknownEnv)
	}
}

func TestBotAPIMultipartPhotoFileFlow(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{Mode: config.ModeLocal, BotToken: "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc", BufferSize: 100}
	handler := NewWithStore(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	imageBytes := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	env := botMultipartCall(t, srv.URL, cfg.BotToken, "sendPhoto", map[string]string{
		"chat_id": "42",
		"caption": "uploaded photo",
	}, "photo", "tiny.png", imageBytes, http.StatusOK)
	msg := decodeBotResult[tg.Message](t, env)
	if len(msg.Photo) != 1 || msg.Photo[0].FileID == "" {
		t.Fatalf("photo sizes = %#v, want uploaded file_id", msg.Photo)
	}
	if !strings.HasPrefix(msg.PhotoURL, "/_sim/file/") {
		t.Fatalf("PhotoURL = %q, want /_sim/file path", msg.PhotoURL)
	}

	fileEnv := botCall(t, srv.URL, cfg.BotToken, "getFile", map[string]any{
		"file_id": msg.Photo[0].FileID,
	}, http.StatusOK)
	file := decodeBotResult[botFileResult](t, fileEnv)
	if file.FileID != msg.Photo[0].FileID || file.FileSize != int64(len(imageBytes)) {
		t.Fatalf("getFile result = %#v, want uploaded file metadata", file)
	}
	if file.FilePath != strings.TrimPrefix(msg.PhotoURL, "/") {
		t.Fatalf("file_path = %q, want %q", file.FilePath, strings.TrimPrefix(msg.PhotoURL, "/"))
	}

	downloadResp, err := http.Get(srv.URL + msg.PhotoURL)
	if err != nil {
		t.Fatalf("download uploaded file failed: %v", err)
	}
	downloaded, err := io.ReadAll(downloadResp.Body)
	_ = downloadResp.Body.Close()
	if err != nil {
		t.Fatalf("read uploaded file response: %v", err)
	}
	if downloadResp.StatusCode != http.StatusOK {
		t.Fatalf("download status = %d, want 200", downloadResp.StatusCode)
	}
	if got := downloadResp.Header.Get("Content-Type"); got != "image/png" {
		t.Fatalf("download content-type = %q, want image/png", got)
	}
	if !bytes.Equal(downloaded, imageBytes) {
		t.Fatalf("downloaded bytes = %q, want original", downloaded)
	}

	resetResp, err := http.Post(srv.URL+"/_sim/reset", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("reset request failed: %v", err)
	}
	_ = resetResp.Body.Close()
	if resetResp.StatusCode != http.StatusOK {
		t.Fatalf("reset status = %d, want 200", resetResp.StatusCode)
	}

	missingResp, err := http.Get(srv.URL + msg.PhotoURL)
	if err != nil {
		t.Fatalf("download after reset failed: %v", err)
	}
	_ = missingResp.Body.Close()
	if missingResp.StatusCode != http.StatusNotFound {
		t.Fatalf("download after reset status = %d, want 404", missingResp.StatusCode)
	}
}

func TestBotAPIGenericMediaUploadFlow(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{Mode: config.ModeLocal, BotToken: "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc", BufferSize: 100}
	handler := NewWithStore(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	documentBytes := []byte("hello document")
	env := botMultipartCall(t, srv.URL, cfg.BotToken, "sendDocument", map[string]string{
		"chat_id": "42",
		"caption": "uploaded document",
	}, "document", "hello.txt", documentBytes, http.StatusOK)
	msg := decodeBotResult[tg.Message](t, env)
	if msg.Document == nil || msg.Document.FileID == "" {
		t.Fatalf("document = %#v, want uploaded document metadata", msg.Document)
	}
	if msg.Document.FileName != "hello.txt" || msg.Document.FileSize != len(documentBytes) {
		t.Fatalf("document metadata = %#v, want file name and size", msg.Document)
	}
	if msg.MediaKind != "document" || !strings.HasPrefix(msg.MediaURL, "/_sim/file/") {
		t.Fatalf("media kind/url = %q/%q, want stored document", msg.MediaKind, msg.MediaURL)
	}

	fileEnv := botCall(t, srv.URL, cfg.BotToken, "getFile", map[string]any{
		"file_id": msg.Document.FileID,
	}, http.StatusOK)
	file := decodeBotResult[botFileResult](t, fileEnv)
	if file.FilePath != strings.TrimPrefix(msg.MediaURL, "/") {
		t.Fatalf("file_path = %q, want %q", file.FilePath, strings.TrimPrefix(msg.MediaURL, "/"))
	}

	downloadResp, err := http.Get(srv.URL + msg.MediaURL)
	if err != nil {
		t.Fatalf("download uploaded document failed: %v", err)
	}
	downloaded, err := io.ReadAll(downloadResp.Body)
	_ = downloadResp.Body.Close()
	if err != nil {
		t.Fatalf("read uploaded document response: %v", err)
	}
	if downloadResp.StatusCode != http.StatusOK {
		t.Fatalf("document download status = %d, want 200", downloadResp.StatusCode)
	}
	if !bytes.Equal(downloaded, documentBytes) {
		t.Fatalf("downloaded document = %q, want original", downloaded)
	}
}

func TestSimCoverageEndpoint(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{Mode: config.ModeLocal, BotToken: "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc", BufferSize: 100}
	handler := NewWithStore(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	coverage := readSimResult[coverageReport](t, srv.URL+"/_sim/coverage")
	if coverage.APIVersion != "10.1" || coverage.APIMode != config.APIModeCompat {
		t.Fatalf("coverage metadata = %#v, want Bot API 10.1 compat", coverage)
	}
	if coverage.Summary.Total == 0 || coverage.Summary.Stateful == 0 || coverage.Summary.NotYetSemantic == 0 {
		t.Fatalf("coverage summary = %#v, want populated levels", coverage.Summary)
	}
	if got := coverage.MethodLevel("sendMessage"); got != "stateful" {
		t.Fatalf("sendMessage level = %q, want stateful", got)
	}
	if got := coverage.MethodLevel("sendVideo"); got != "ui_rendered" {
		t.Fatalf("sendVideo level = %q, want ui_rendered", got)
	}
	if got := coverage.MethodLevel("sendInvoice"); got != "not_yet_semantic" {
		t.Fatalf("sendInvoice level = %q, want not_yet_semantic", got)
	}
}

func TestBotAPIStrictModeRejectsNonSemanticMethods(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{Mode: config.ModeLocal, BotToken: "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc", APIMode: config.APIModeStrict, BufferSize: 100}
	handler := NewWithStore(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	okEnv := botCall(t, srv.URL, cfg.BotToken, "sendMessage", map[string]any{
		"chat_id": 42,
		"text":    "strict mode still allows stateful methods",
	}, http.StatusOK)
	if !okEnv.OK {
		t.Fatalf("sendMessage response = %#v, want ok", okEnv)
	}

	rejectedEnv := botCall(t, srv.URL, cfg.BotToken, "sendInvoice", map[string]any{
		"chat_id": 42,
		"title":   "Demo invoice",
	}, http.StatusNotImplemented)
	if rejectedEnv.OK || rejectedEnv.ErrorCode != http.StatusNotImplemented || !strings.Contains(rejectedEnv.Description, "strict api mode") {
		t.Fatalf("sendInvoice strict response = %#v, want strict 501", rejectedEnv)
	}

	coverage := readSimResult[coverageReport](t, srv.URL+"/_sim/coverage")
	if coverage.APIMode != config.APIModeStrict {
		t.Fatalf("coverage api mode = %q, want strict", coverage.APIMode)
	}
}

func TestBotAPIChatActionAndDraftEvents(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{Mode: config.ModeLocal, BotToken: "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc", BufferSize: 100}
	handler := NewWithStore(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	events, stopSSE := startSSE(t, srv.URL)
	t.Cleanup(stopSSE)

	actionEnv := botCall(t, srv.URL, cfg.BotToken, "sendChatAction", map[string]any{
		"chat_id": 42,
		"action":  "typing",
	}, http.StatusOK)
	if !actionEnv.OK {
		t.Fatalf("sendChatAction response = %#v, want ok", actionEnv)
	}
	action := waitEventPayload[map[string]any](t, events, "chat_action", func(payload map[string]any) bool {
		return payload["action"] == "typing"
	})
	if action["chat_id"].(float64) != 42 {
		t.Fatalf("chat_action payload = %#v, want chat 42", action)
	}

	draftEnv := botCall(t, srv.URL, cfg.BotToken, "sendMessageDraft", map[string]any{
		"chat_id":  42,
		"draft_id": 7,
		"text":     "Печатаю ответ",
	}, http.StatusOK)
	if !draftEnv.OK {
		t.Fatalf("sendMessageDraft response = %#v, want ok", draftEnv)
	}
	draft := waitEventPayload[map[string]any](t, events, "message_draft", func(payload map[string]any) bool {
		return payload["text"] == "Печатаю ответ"
	})
	if draft["draft_id"].(float64) != 7 {
		t.Fatalf("draft payload = %#v, want draft 7", draft)
	}
}

func TestSimResetClearsStateAndTraces(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{Mode: config.ModeLocal, BotToken: "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc", BufferSize: 100}
	handler := NewWithStore(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	bot, err := telego.NewBot(cfg.BotToken, telego.WithAPIServer(srv.URL), telego.WithDiscardLogger())
	if err != nil {
		t.Fatalf("NewBot returned error: %v", err)
	}

	resp, err := http.Post(srv.URL+"/_sim/inject", "application/json", strings.NewReader(`{"type":"message","chat_id":42,"user_id":7,"text":"/start"}`))
	if err != nil {
		t.Fatalf("inject request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("inject status = %d, want 200", resp.StatusCode)
	}

	updates, err := bot.GetUpdates(&telego.GetUpdatesParams{Limit: 10, Timeout: 1})
	if err != nil {
		t.Fatalf("GetUpdates returned error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("updates length = %d, want 1", len(updates))
	}
	if _, err := bot.SendMessage(&telego.SendMessageParams{ChatID: telego.ChatID{ID: 42}, Text: "before reset"}); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	tracesBefore := readSimResult[[]any](t, srv.URL+"/_sim/traces")
	if len(tracesBefore) == 0 {
		t.Fatal("traces before reset length = 0, want at least 1")
	}

	resetResp, err := http.Post(srv.URL+"/_sim/reset", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("reset request failed: %v", err)
	}
	_ = resetResp.Body.Close()
	if resetResp.StatusCode != http.StatusOK {
		t.Fatalf("reset status = %d, want 200", resetResp.StatusCode)
	}

	state, err := st.State(context.Background())
	if err != nil {
		t.Fatalf("State returned error: %v", err)
	}
	if len(state.Chats) != 0 || len(state.Messages) != 0 {
		t.Fatalf("state after reset = %#v, want empty", state)
	}
	tracesAfter := readSimResult[[]any](t, srv.URL+"/_sim/traces")
	if len(tracesAfter) != 0 {
		t.Fatalf("traces after reset length = %d, want 0", len(tracesAfter))
	}

	updatesAfter, err := bot.GetUpdates(&telego.GetUpdatesParams{Limit: 10, Timeout: 0})
	if err != nil {
		t.Fatalf("GetUpdates after reset returned error: %v", err)
	}
	if len(updatesAfter) != 0 {
		t.Fatalf("updates after reset length = %d, want 0", len(updatesAfter))
	}
}

func TestSimTraceResetKeepsChatState(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{Mode: config.ModeLocal, BotToken: "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc", BufferSize: 100}
	handler := NewWithStore(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	bot, err := telego.NewBot(cfg.BotToken, telego.WithAPIServer(srv.URL), telego.WithDiscardLogger())
	if err != nil {
		t.Fatalf("NewBot returned error: %v", err)
	}

	resp, err := http.Post(srv.URL+"/_sim/inject", "application/json", strings.NewReader(`{"type":"message","chat_id":42,"user_id":7,"text":"/start"}`))
	if err != nil {
		t.Fatalf("inject request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("inject status = %d, want 200", resp.StatusCode)
	}

	updates, err := bot.GetUpdates(&telego.GetUpdatesParams{Limit: 10, Timeout: 1})
	if err != nil {
		t.Fatalf("GetUpdates returned error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("updates length = %d, want 1", len(updates))
	}
	if _, err := bot.SendMessage(&telego.SendMessageParams{ChatID: telego.ChatID{ID: 42}, Text: "keep this message"}); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	tracesBefore := readSimResult[[]any](t, srv.URL+"/_sim/traces")
	if len(tracesBefore) == 0 {
		t.Fatal("traces before trace reset length = 0, want at least 1")
	}

	resetResp, err := http.Post(srv.URL+"/_sim/traces/reset", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("trace reset request failed: %v", err)
	}
	_ = resetResp.Body.Close()
	if resetResp.StatusCode != http.StatusOK {
		t.Fatalf("trace reset status = %d, want 200", resetResp.StatusCode)
	}

	tracesAfter := readSimResult[[]any](t, srv.URL+"/_sim/traces")
	if len(tracesAfter) != 0 {
		t.Fatalf("traces after trace reset length = %d, want 0", len(tracesAfter))
	}

	state, err := st.State(context.Background())
	if err != nil {
		t.Fatalf("State returned error: %v", err)
	}
	messages := state.Messages["42"]
	if len(messages) != 2 {
		t.Fatalf("messages after trace reset length = %d, want 2", len(messages))
	}
	if messages[0].Text != "/start" || messages[1].Text != "keep this message" {
		t.Fatalf("messages after trace reset = %q, %q", messages[0].Text, messages[1].Text)
	}
}

func TestSimResetPreservesWebhookConfig(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{Mode: config.ModeLocal, BotToken: "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc", BufferSize: 100}
	handler := NewWithStore(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(webhookSrv.Close)

	bot, err := telego.NewBot(cfg.BotToken, telego.WithAPIServer(srv.URL), telego.WithDiscardLogger())
	if err != nil {
		t.Fatalf("NewBot returned error: %v", err)
	}
	if err := bot.SetWebhook(&telego.SetWebhookParams{URL: webhookSrv.URL}); err != nil {
		t.Fatalf("SetWebhook returned error: %v", err)
	}

	resetResp, err := http.Post(srv.URL+"/_sim/reset", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("reset request failed: %v", err)
	}
	_ = resetResp.Body.Close()
	if resetResp.StatusCode != http.StatusOK {
		t.Fatalf("reset status = %d, want 200", resetResp.StatusCode)
	}

	info, err := bot.GetWebhookInfo()
	if err != nil {
		t.Fatalf("GetWebhookInfo returned error: %v", err)
	}
	if info.URL != webhookSrv.URL {
		t.Fatalf("webhook URL after reset = %q, want %q", info.URL, webhookSrv.URL)
	}
}

type botAPIEnvelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
	Description string          `json:"description,omitempty"`
}

type messageEventPayload struct {
	Op      string     `json:"op"`
	Message tg.Message `json:"message"`
}

type callbackAnswerEventPayload struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
	ShowAlert       bool   `json:"show_alert,omitempty"`
}

type coverageReport struct {
	APIVersion string `json:"api_version"`
	APIMode    string `json:"api_mode"`
	Summary    struct {
		Total          int `json:"total"`
		Stateful       int `json:"stateful"`
		NotYetSemantic int `json:"not_yet_semantic"`
	} `json:"summary"`
	Methods []struct {
		Name  string `json:"name"`
		Level string `json:"level"`
	} `json:"methods"`
}

func (report coverageReport) MethodLevel(name string) string {
	for _, method := range report.Methods {
		if method.Name == name {
			return method.Level
		}
	}
	return ""
}

type botFileResult struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileSize     int64  `json:"file_size"`
	FilePath     string `json:"file_path"`
}

func botCall(t *testing.T, baseURL, token, method string, body any, wantStatus int) botAPIEnvelope {
	t.Helper()

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal %s body: %v", method, err)
	}
	req, err := http.NewRequest(http.MethodPost, baseURL+"/bot"+token+"/"+method, bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("create %s request: %v", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s request failed: %v", method, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s response: %v", method, err)
	}
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s status = %d, want %d, body = %s", method, resp.StatusCode, wantStatus, respBody)
	}

	var env botAPIEnvelope
	if err := json.Unmarshal(respBody, &env); err != nil {
		t.Fatalf("decode %s response %q: %v", method, respBody, err)
	}
	return env
}

func botMultipartCall(t *testing.T, baseURL, token, method string, fields map[string]string, fileField, fileName string, data []byte, wantStatus int) botAPIEnvelope {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write multipart field %s: %v", key, err)
		}
	}
	part, err := writer.CreateFormFile(fileField, fileName)
	if err != nil {
		t.Fatalf("create multipart file %s: %v", fileField, err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write multipart file %s: %v", fileField, err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL+"/bot"+token+"/"+method, &body)
	if err != nil {
		t.Fatalf("create %s multipart request: %v", method, err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s multipart request failed: %v", method, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s multipart response: %v", method, err)
	}
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s multipart status = %d, want %d, body = %s", method, resp.StatusCode, wantStatus, respBody)
	}

	var env botAPIEnvelope
	if err := json.Unmarshal(respBody, &env); err != nil {
		t.Fatalf("decode %s multipart response %q: %v", method, respBody, err)
	}
	return env
}

func decodeBotResult[T any](t *testing.T, env botAPIEnvelope) T {
	t.Helper()

	var out T
	if len(env.Result) == 0 {
		t.Fatalf("empty result in response %#v", env)
	}
	if err := json.Unmarshal(env.Result, &out); err != nil {
		t.Fatalf("decode result %q: %v", env.Result, err)
	}
	return out
}

func rawJSONContains(raw json.RawMessage, needle string) bool {
	return strings.Contains(string(raw), needle)
}

func readSimResult[T any](t *testing.T, url string) T {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s response: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, body = %s", url, resp.StatusCode, body)
	}
	var env botAPIEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode %s response %q: %v", url, body, err)
	}
	return decodeBotResult[T](t, env)
}
