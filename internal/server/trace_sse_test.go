package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mymmrac/telego"

	"github.com/Zulut30/local-telegram-client/internal/config"
	"github.com/Zulut30/local-telegram-client/internal/store"
	"github.com/Zulut30/local-telegram-client/internal/tg"
	tracing "github.com/Zulut30/local-telegram-client/internal/trace"
	"github.com/Zulut30/local-telegram-client/internal/webhook"
)

func TestSSETraceCorrelation(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{Mode: config.ModeLocal, BotToken: "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc", BufferSize: 100}
	handler := NewWithStore(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	events, stopSSE := startSSE(t, srv.URL)
	t.Cleanup(stopSSE)

	bot, err := telego.NewBot(cfg.BotToken, telego.WithAPIServer(srv.URL), telego.WithDiscardLogger())
	if err != nil {
		t.Fatalf("NewBot returned error: %v", err)
	}

	injectBody := bytes.NewBufferString(`{"type":"message","chat_id":42,"user_id":7,"username":"dev","text":"/start"}`)
	resp, err := http.Post(srv.URL+"/_sim/inject", "application/json", injectBody)
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

	openTrace := waitTraceEvent(t, events, func(payload tracing.EventPayload) bool {
		return payload.Op == "open" &&
			payload.Trace.Inbound != nil &&
			payload.Trace.Inbound.UpdateID == int64(updates[0].UpdateID) &&
			payload.Trace.Inbound.ChatID == 42
	})
	if openTrace.Trace.Correlation != tracing.CorrelationInferred {
		t.Fatalf("trace correlation = %q, want inferred", openTrace.Trace.Correlation)
	}

	if _, err := bot.SendMessage(&telego.SendMessageParams{
		ChatID: telego.ChatID{ID: updates[0].Message.Chat.ID},
		Text:   "hello from bot",
	}); err != nil {
		t.Fatalf("SendMessage returned error: %v", err)
	}

	updatedTrace := waitTraceEvent(t, events, func(payload tracing.EventPayload) bool {
		return payload.Op == "update" &&
			payload.Trace.ID == openTrace.Trace.ID &&
			len(payload.Trace.Calls) == 1
	})
	call := updatedTrace.Trace.Calls[0]
	if call.Method != "sendMessage" || !call.OK || call.HTTPStatus != http.StatusOK {
		t.Fatalf("call = %#v, want successful sendMessage", call)
	}
	if call.Correlation != tracing.CorrelationInferred {
		t.Fatalf("call correlation = %q, want inferred", call.Correlation)
	}

	if _, err := bot.GetUpdates(&telego.GetUpdatesParams{Offset: updates[0].UpdateID + 1}); err != nil {
		t.Fatalf("flush GetUpdates returned error: %v", err)
	}
	closedTrace := waitTraceEvent(t, events, func(payload tracing.EventPayload) bool {
		return payload.Op == "close" && payload.Trace.ID == openTrace.Trace.ID
	})
	if closedTrace.Trace.Status != tracing.StatusOK {
		t.Fatalf("closed trace status = %q, want ok", closedTrace.Trace.Status)
	}

	badResp, err := http.Post(
		srv.URL+"/bot"+cfg.BotToken+"/sendMessage",
		"application/json",
		strings.NewReader(`{"chat_id":42}`),
	)
	if err != nil {
		t.Fatalf("bad sendMessage request failed: %v", err)
	}
	_ = badResp.Body.Close()
	if badResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad sendMessage status = %d, want 400", badResp.StatusCode)
	}

	errorTrace := waitTraceEvent(t, events, func(payload tracing.EventPayload) bool {
		return payload.Op == "close" &&
			payload.Trace.Orphan &&
			payload.Trace.Status == tracing.StatusError &&
			len(payload.Trace.Calls) == 1 &&
			payload.Trace.Calls[0].Method == "sendMessage"
	})
	if errorTrace.Trace.Calls[0].OK {
		t.Fatalf("error call OK = true, want false")
	}
	if errorTrace.Trace.Calls[0].ErrorCode != 400 {
		t.Fatalf("error code = %d, want 400", errorTrace.Trace.Calls[0].ErrorCode)
	}
}

func TestSSECallbackTraceCorrelation(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{Mode: config.ModeLocal, BotToken: "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc", BufferSize: 100}
	handler := NewWithStore(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	events, stopSSE := startSSE(t, srv.URL)
	t.Cleanup(stopSSE)

	sendEnv := botCall(t, srv.URL, cfg.BotToken, "sendMessage", map[string]any{
		"chat_id": 42,
		"text":    "Choose",
		"reply_markup": map[string]any{
			"inline_keyboard": [][]map[string]string{{
				{"text": "Open", "callback_data": "open"},
			}},
		},
	}, http.StatusOK)
	sent := decodeBotResult[tg.Message](t, sendEnv)
	_ = waitEventPayload[messageEventPayload](t, events, "message", func(payload messageEventPayload) bool {
		return payload.Op == "created" && payload.Message.MessageID == sent.MessageID
	})

	bot, err := telego.NewBot(cfg.BotToken, telego.WithAPIServer(srv.URL), telego.WithDiscardLogger())
	if err != nil {
		t.Fatalf("NewBot returned error: %v", err)
	}

	injectPayload, err := json.Marshal(map[string]any{
		"type":       "callback_query",
		"chat_id":    42,
		"message_id": sent.MessageID,
		"user_id":    7,
		"username":   "dev",
		"data":       "open",
	})
	if err != nil {
		t.Fatalf("marshal inject payload: %v", err)
	}
	resp, err := http.Post(srv.URL+"/_sim/inject", "application/json", bytes.NewReader(injectPayload))
	if err != nil {
		t.Fatalf("inject callback request failed: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("inject callback status = %d, want 200", resp.StatusCode)
	}

	updates, err := bot.GetUpdates(&telego.GetUpdatesParams{Limit: 10, Timeout: 1})
	if err != nil {
		t.Fatalf("GetUpdates returned error: %v", err)
	}
	if len(updates) != 1 || updates[0].CallbackQuery == nil {
		t.Fatalf("updates = %#v, want one callback query", updates)
	}
	callbackID := updates[0].CallbackQuery.ID

	openTrace := waitTraceEvent(t, events, func(payload tracing.EventPayload) bool {
		return payload.Op == "open" &&
			payload.Trace.Inbound != nil &&
			payload.Trace.Inbound.Type == "callback_query" &&
			payload.Trace.Inbound.CallbackQueryID == callbackID
	})
	if openTrace.Trace.Orphan {
		t.Fatal("callback trace is orphan, want inbound-correlated trace")
	}

	answerEnv := botCall(t, srv.URL, cfg.BotToken, "answerCallbackQuery", map[string]any{
		"callback_query_id": callbackID,
		"text":              "Opened",
	}, http.StatusOK)
	if ok := decodeBotResult[bool](t, answerEnv); !ok {
		t.Fatal("answerCallbackQuery result = false, want true")
	}

	editEnv := botCall(t, srv.URL, cfg.BotToken, "editMessageText", map[string]any{
		"chat_id":    42,
		"message_id": sent.MessageID,
		"text":       "Opened",
	}, http.StatusOK)
	edited := decodeBotResult[tg.Message](t, editEnv)
	if edited.Text != "Opened" {
		t.Fatalf("edited text = %q, want Opened", edited.Text)
	}

	updatedTrace := waitTraceEvent(t, events, func(payload tracing.EventPayload) bool {
		return payload.Op == "update" &&
			payload.Trace.ID == openTrace.Trace.ID &&
			traceHasCalls(payload.Trace, "answerCallbackQuery", "editMessageText")
	})
	if updatedTrace.Trace.Orphan {
		t.Fatal("updated callback trace is orphan")
	}
	for _, call := range updatedTrace.Trace.Calls {
		if call.Method == "answerCallbackQuery" && call.Correlation != tracing.CorrelationInferred {
			t.Fatalf("answerCallbackQuery correlation = %q, want inferred", call.Correlation)
		}
	}

	if _, err := bot.GetUpdates(&telego.GetUpdatesParams{Offset: updates[0].UpdateID + 1}); err != nil {
		t.Fatalf("flush GetUpdates returned error: %v", err)
	}
	closedTrace := waitTraceEvent(t, events, func(payload tracing.EventPayload) bool {
		return payload.Op == "close" && payload.Trace.ID == openTrace.Trace.ID
	})
	if closedTrace.Trace.Status != tracing.StatusOK {
		t.Fatalf("closed trace status = %q, want ok", closedTrace.Trace.Status)
	}
}

func TestWebhookDeliveryAndTraceCorrelation(t *testing.T) {
	st := store.NewMemory()
	cfg := config.Config{Mode: config.ModeLocal, BotToken: "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc", BufferSize: 100}
	handler := NewWithStore(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), st)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	events, stopSSE := startSSE(t, srv.URL)
	t.Cleanup(stopSSE)

	received := make(chan tg.Update, 1)
	handlerErrs := make(chan error, 1)
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var update tg.Update
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, "decode update", http.StatusBadRequest)
			return
		}
		select {
		case received <- update:
		default:
		}
		if update.Message != nil {
			err := postWebhookBotCall(srv.URL, cfg.BotToken, "sendMessage", map[string]any{
				"chat_id": update.Message.Chat.ID,
				"text":    "webhook pong",
			})
			if err != nil {
				select {
				case handlerErrs <- err:
				default:
				}
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(webhookSrv.Close)

	setEnv := botCall(t, srv.URL, cfg.BotToken, "setWebhook", map[string]any{
		"url": webhookSrv.URL,
	}, http.StatusOK)
	if ok := decodeBotResult[bool](t, setEnv); !ok {
		t.Fatal("setWebhook result = false, want true")
	}

	infoEnv := botCall(t, srv.URL, cfg.BotToken, "getWebhookInfo", map[string]any{}, http.StatusOK)
	info := decodeBotResult[webhook.Info](t, infoEnv)
	if info.URL != webhookSrv.URL {
		t.Fatalf("webhook info URL = %q, want %q", info.URL, webhookSrv.URL)
	}

	conflictEnv := botCall(t, srv.URL, cfg.BotToken, "getUpdates", map[string]any{"timeout": 0}, http.StatusConflict)
	if conflictEnv.OK || conflictEnv.ErrorCode != http.StatusConflict {
		t.Fatalf("getUpdates conflict response = %#v, want 409 error", conflictEnv)
	}

	injectBody := bytes.NewBufferString(`{"type":"message","chat_id":77,"user_id":7,"username":"dev","text":"/hook"}`)
	resp, err := http.Post(srv.URL+"/_sim/inject", "application/json", injectBody)
	if err != nil {
		t.Fatalf("inject webhook message request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("inject webhook status = %d, body = %s", resp.StatusCode, body)
	}

	delivered := waitWebhookUpdate(t, received)
	if delivered.Message == nil || delivered.Message.Text != "/hook" || delivered.Message.Chat.ID != 77 {
		t.Fatalf("delivered update = %#v, want /hook message for chat 77", delivered)
	}
	select {
	case err := <-handlerErrs:
		t.Fatalf("webhook handler bot call failed: %v", err)
	default:
	}

	openTrace := waitTraceEvent(t, events, func(payload tracing.EventPayload) bool {
		return payload.Op == "open" &&
			payload.Trace.Inbound != nil &&
			payload.Trace.Inbound.Type == "message" &&
			payload.Trace.Inbound.UpdateID == delivered.UpdateID &&
			payload.Trace.Inbound.ChatID == 77
	})
	updatedTrace := waitTraceEvent(t, events, func(payload tracing.EventPayload) bool {
		return payload.Op == "update" &&
			payload.Trace.ID == openTrace.Trace.ID &&
			traceHasCalls(payload.Trace, "sendMessage")
	})
	if updatedTrace.Trace.Calls[0].Correlation != tracing.CorrelationInferred {
		t.Fatalf("webhook call correlation = %q, want inferred", updatedTrace.Trace.Calls[0].Correlation)
	}
	closedTrace := waitTraceEvent(t, events, func(payload tracing.EventPayload) bool {
		return payload.Op == "close" && payload.Trace.ID == openTrace.Trace.ID
	})
	if closedTrace.Trace.Status != tracing.StatusOK {
		t.Fatalf("webhook trace status = %q, want ok", closedTrace.Trace.Status)
	}

	state, err := st.State(context.Background())
	if err != nil {
		t.Fatalf("State returned error: %v", err)
	}
	messages := state.Messages["77"]
	if len(messages) != 2 {
		encoded, _ := json.Marshal(messages)
		t.Fatalf("stored webhook messages length = %d, want 2: %s", len(messages), encoded)
	}
	if messages[0].Text != "/hook" || messages[1].Text != "webhook pong" {
		t.Fatalf("stored webhook texts = %q, %q", messages[0].Text, messages[1].Text)
	}

	deleteEnv := botCall(t, srv.URL, cfg.BotToken, "deleteWebhook", map[string]any{}, http.StatusOK)
	if ok := decodeBotResult[bool](t, deleteEnv); !ok {
		t.Fatal("deleteWebhook result = false, want true")
	}
	emptyInfoEnv := botCall(t, srv.URL, cfg.BotToken, "getWebhookInfo", map[string]any{}, http.StatusOK)
	emptyInfo := decodeBotResult[webhook.Info](t, emptyInfoEnv)
	if emptyInfo.URL != "" {
		t.Fatalf("webhook info URL after delete = %q, want empty", emptyInfo.URL)
	}

	pollEnv := botCall(t, srv.URL, cfg.BotToken, "getUpdates", map[string]any{"limit": 10, "timeout": 0}, http.StatusOK)
	polled := decodeBotResult[[]tg.Update](t, pollEnv)
	if len(polled) != 0 {
		t.Fatalf("polled updates after webhook ack = %d, want 0", len(polled))
	}
}

type sseEvent struct {
	name string
	data []byte
}

func startSSE(t *testing.T, baseURL string) (<-chan sseEvent, context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/_sim/events", nil)
	if err != nil {
		cancel()
		t.Fatalf("create SSE request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("start SSE request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		_ = resp.Body.Close()
		t.Fatalf("SSE status = %d, want 200", resp.StatusCode)
	}

	ch := make(chan sseEvent, 32)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		reader := bufio.NewReader(resp.Body)
		var name string
		var data []string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				if name != "" {
					ev := sseEvent{name: name, data: []byte(strings.Join(data, "\n"))}
					select {
					case ch <- ev:
					case <-ctx.Done():
						return
					}
				}
				name = ""
				data = nil
				continue
			}
			if value, ok := strings.CutPrefix(line, "event:"); ok {
				name = strings.TrimSpace(value)
			}
			if value, ok := strings.CutPrefix(line, "data:"); ok {
				data = append(data, strings.TrimSpace(value))
			}
		}
	}()

	return ch, cancel
}

func waitEventPayload[T any](t *testing.T, events <-chan sseEvent, name string, match func(T) bool) T {
	t.Helper()

	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-timeout:
			var zero T
			t.Fatalf("timed out waiting for %s SSE event", name)
			return zero
		case ev, ok := <-events:
			if !ok {
				var zero T
				t.Fatal("SSE stream closed")
				return zero
			}
			if ev.name != name {
				continue
			}
			var payload T
			if err := json.Unmarshal(ev.data, &payload); err != nil {
				t.Fatalf("decode %s event %q: %v", name, ev.data, err)
			}
			if match == nil || match(payload) {
				return payload
			}
		}
	}
}

func waitTraceEvent(t *testing.T, events <-chan sseEvent, match func(tracing.EventPayload) bool) tracing.EventPayload {
	t.Helper()

	timeout := time.After(2 * time.Second)
	for {
		select {
		case <-timeout:
			t.Fatal("timed out waiting for trace SSE event")
		case ev, ok := <-events:
			if !ok {
				t.Fatal("SSE stream closed")
			}
			if ev.name != "trace" {
				continue
			}
			var payload tracing.EventPayload
			if err := json.Unmarshal(ev.data, &payload); err != nil {
				t.Fatalf("decode trace event %q: %v", ev.data, err)
			}
			if match(payload) {
				return payload
			}
		}
	}
}

func traceHasCalls(trace tracing.Trace, methods ...string) bool {
	found := make(map[string]bool, len(methods))
	for _, call := range trace.Calls {
		found[call.Method] = true
	}
	for _, method := range methods {
		if !found[method] {
			return false
		}
	}
	return true
}

func waitWebhookUpdate(t *testing.T, ch <-chan tg.Update) tg.Update {
	t.Helper()

	select {
	case update := <-ch:
		return update
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for webhook update")
		return tg.Update{}
	}
}

func postWebhookBotCall(baseURL, token, method string, body any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := http.Post(baseURL+"/bot"+token+"/"+method, "application/json", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned HTTP %d: %s", method, resp.StatusCode, respBody)
	}
	var env botAPIEnvelope
	if err := json.Unmarshal(respBody, &env); err != nil {
		return err
	}
	if !env.OK {
		return fmt.Errorf("%s returned error %d: %s", method, env.ErrorCode, env.Description)
	}
	return nil
}
