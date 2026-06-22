package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mymmrac/telego"

	"github.com/Zulut30/local-telegram-client/internal/config"
	"github.com/Zulut30/local-telegram-client/internal/store"
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
