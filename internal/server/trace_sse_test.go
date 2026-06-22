package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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
	tracing "github.com/Zulut30/local-telegram-client/internal/trace"
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
