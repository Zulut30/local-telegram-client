package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/mymmrac/telego"

	"github.com/Zulut30/local-telegram-client/internal/config"
	"github.com/Zulut30/local-telegram-client/internal/showcase"
	"github.com/Zulut30/local-telegram-client/internal/store"
	tracing "github.com/Zulut30/local-telegram-client/internal/trace"
)

func TestShowcasePollingSmoke(t *testing.T) {
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
	app := showcase.New(bot, showcase.NewTraceErrorTrigger(srv.URL, cfg.BotToken))

	injectTextForShowcase(t, srv.URL, 42, "/start")

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
			payload.Trace.Inbound.UpdateID == int64(updates[0].UpdateID)
	})
	if err := app.Handle(updates[0]); err != nil {
		t.Fatalf("showcase Handle returned error: %v", err)
	}

	_ = waitTraceEvent(t, events, func(payload tracing.EventPayload) bool {
		return payload.Op == "update" &&
			payload.Trace.ID == openTrace.Trace.ID &&
			traceHasCalls(payload.Trace, "sendMessage")
	})
	assertShowcaseMessage(t, st, 42)
}

func TestShowcaseWebhookSmoke(t *testing.T) {
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
	app := showcase.New(bot, showcase.NewTraceErrorTrigger(srv.URL, cfg.BotToken))
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var update telego.Update
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, "decode update", http.StatusBadRequest)
			return
		}
		if err := app.Handle(update); err != nil {
			http.Error(w, "handle update", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(webhookSrv.Close)

	if err := bot.SetWebhook(&telego.SetWebhookParams{URL: webhookSrv.URL}); err != nil {
		t.Fatalf("SetWebhook returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = bot.DeleteWebhook(&telego.DeleteWebhookParams{})
	})

	injectTextForShowcase(t, srv.URL, 77, "/start")

	openTrace := waitTraceEvent(t, events, func(payload tracing.EventPayload) bool {
		return payload.Op == "open" &&
			payload.Trace.Inbound != nil &&
			payload.Trace.Inbound.Type == "message" &&
			payload.Trace.Inbound.ChatID == 77
	})
	_ = waitTraceEvent(t, events, func(payload tracing.EventPayload) bool {
		return payload.Op == "update" &&
			payload.Trace.ID == openTrace.Trace.ID &&
			traceHasCalls(payload.Trace, "sendMessage")
	})
	closedTrace := waitTraceEvent(t, events, func(payload tracing.EventPayload) bool {
		return payload.Op == "close" && payload.Trace.ID == openTrace.Trace.ID
	})
	if closedTrace.Trace.Status != tracing.StatusOK {
		t.Fatalf("closed trace status = %q, want ok", closedTrace.Trace.Status)
	}
	assertShowcaseMessage(t, st, 77)

	if err := bot.DeleteWebhook(&telego.DeleteWebhookParams{}); err != nil {
		t.Fatalf("DeleteWebhook returned error: %v", err)
	}
	updates, err := bot.GetUpdates(&telego.GetUpdatesParams{Limit: 10, Timeout: 0})
	if err != nil {
		t.Fatalf("GetUpdates returned error: %v", err)
	}
	if len(updates) != 0 {
		t.Fatalf("updates length after webhook delivery = %d, want 0", len(updates))
	}
}

func injectTextForShowcase(t *testing.T, baseURL string, chatID int64, text string) {
	t.Helper()

	resp, err := http.Post(baseURL+"/_sim/inject", "application/json", strings.NewReader(
		`{"type":"message","chat_id":`+strconvFormatInt(chatID)+`,"user_id":7,"username":"dev","text":`+strconvQuote(text)+`}`,
	))
	if err != nil {
		t.Fatalf("inject request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("inject status = %d, body = %s", resp.StatusCode, body)
	}
}

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

func strconvQuote(value string) string {
	return strconv.Quote(value)
}

func assertShowcaseMessage(t *testing.T, st *store.Memory, chatID int64) {
	t.Helper()

	state, err := st.State(context.Background())
	if err != nil {
		t.Fatalf("State returned error: %v", err)
	}
	messages := state.Messages[strconv.FormatInt(chatID, 10)]
	for _, message := range messages {
		if strings.Contains(message.Text, "Recipe bot is ready") {
			return
		}
	}
	t.Fatalf("chat %d messages = %#v, want showcase start message", chatID, messages)
}
