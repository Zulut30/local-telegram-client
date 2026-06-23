package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
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
