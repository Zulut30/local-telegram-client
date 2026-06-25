package trace

import (
	"testing"

	"github.com/Zulut30/local-telegram-client/internal/tg"
)

func messageUpdate(updateID, chatID int64, text string) tg.Update {
	return tg.Update{
		UpdateID: updateID,
		Message: &tg.Message{
			MessageID: updateID,
			Chat:      tg.Chat{ID: chatID, Type: "private"},
			Text:      text,
		},
	}
}

func callbackUpdate(updateID, chatID int64, cbID, data string) tg.Update {
	return tg.Update{
		UpdateID: updateID,
		CallbackQuery: &tg.CallbackQuery{
			ID:      cbID,
			Data:    data,
			Message: &tg.Message{MessageID: updateID, Chat: tg.Chat{ID: chatID, Type: "private"}},
		},
	}
}

func okCall(method string) OutboundCall {
	return OutboundCall{Method: method, OK: true, HTTPStatus: 200}
}
func errCall(method string) OutboundCall {
	return OutboundCall{Method: method, OK: false, HTTPStatus: 400, ErrorCode: 400}
}

func TestRecordCallCorrelatesByChat(t *testing.T) {
	r := NewRecorder(10, nil)
	r.OpenForUpdates([]tg.Update{messageUpdate(1, 7, "/start")})

	chat := int64(7)
	r.RecordCall(&chat, "", okCall("sendMessage"))

	traces := r.Snapshot()
	if len(traces) != 1 {
		t.Fatalf("want 1 trace, got %d", len(traces))
	}
	tr := traces[0]
	if tr.Inbound == nil || tr.Inbound.ChatID != 7 {
		t.Fatalf("inbound not set for chat 7: %+v", tr.Inbound)
	}
	if len(tr.Calls) != 1 || tr.Calls[0].Method != "sendMessage" {
		t.Fatalf("want one sendMessage call, got %+v", tr.Calls)
	}
	if tr.Calls[0].Correlation != CorrelationInferred {
		t.Fatalf("call correlation = %q, want inferred", tr.Calls[0].Correlation)
	}
	if tr.Status != StatusOpen {
		t.Fatalf("status = %q, want open before flush", tr.Status)
	}
}

func TestFlushOpenClosesTraceWithOK(t *testing.T) {
	r := NewRecorder(10, nil)
	r.OpenForUpdates([]tg.Update{messageUpdate(1, 7, "/start")})
	chat := int64(7)
	r.RecordCall(&chat, "", okCall("sendMessage"))

	r.FlushOpen()

	tr := r.Snapshot()[0]
	if tr.Status != StatusOK {
		t.Fatalf("status = %q, want ok after flush", tr.Status)
	}
	if tr.FinishedAt == nil {
		t.Fatal("FinishedAt not set after flush")
	}
}

func TestErrorCallMarksTraceError(t *testing.T) {
	r := NewRecorder(10, nil)
	r.OpenForUpdates([]tg.Update{messageUpdate(1, 7, "/start")})
	chat := int64(7)
	r.RecordCall(&chat, "", errCall("sendMessage"))
	r.FlushOpen()

	if got := r.Snapshot()[0].Status; got != StatusError {
		t.Fatalf("status = %q, want error", got)
	}
}

func TestOrphanCallWhenNoActiveWindow(t *testing.T) {
	r := NewRecorder(10, nil)
	chat := int64(99)
	r.RecordCall(&chat, "", okCall("sendMessage"))

	traces := r.Snapshot()
	if len(traces) != 1 {
		t.Fatalf("want 1 orphan trace, got %d", len(traces))
	}
	tr := traces[0]
	if !tr.Orphan {
		t.Fatal("trace should be marked orphan")
	}
	if tr.Inbound != nil {
		t.Fatalf("orphan trace should have no inbound, got %+v", tr.Inbound)
	}
	if tr.Status != StatusOK || tr.FinishedAt == nil {
		t.Fatalf("orphan should be finished ok, got status %q finished %v", tr.Status, tr.FinishedAt)
	}
}

func TestCallbackCorrelatesByCallbackQueryID(t *testing.T) {
	r := NewRecorder(10, nil)
	r.OpenForUpdates([]tg.Update{callbackUpdate(1, 7, "cb_1", "open:1")})

	// No chat match on purpose: correlate purely by callback query id.
	r.RecordCall(nil, "cb_1", okCall("answerCallbackQuery"))

	tr := r.Snapshot()[0]
	if tr.Inbound == nil || tr.Inbound.Type != "callback_query" {
		t.Fatalf("inbound type = %+v, want callback_query", tr.Inbound)
	}
	if len(tr.Calls) != 1 {
		t.Fatalf("want 1 call attached by cb id, got %d", len(tr.Calls))
	}
}

func TestRingBufferEvictsOldestTraces(t *testing.T) {
	r := NewRecorder(2, nil)
	r.OpenForUpdates([]tg.Update{
		messageUpdate(1, 1, "a"),
		messageUpdate(2, 2, "b"),
		messageUpdate(3, 3, "c"),
	})

	traces := r.Snapshot()
	if len(traces) != 2 {
		t.Fatalf("ring should cap at 2, got %d", len(traces))
	}
	if traces[0].Inbound.ChatID != 2 || traces[1].Inbound.ChatID != 3 {
		t.Fatalf("oldest trace not evicted: %d, %d", traces[0].Inbound.ChatID, traces[1].Inbound.ChatID)
	}
}

func TestWebhookOpenAndClose(t *testing.T) {
	r := NewRecorder(10, nil)
	id, ok := r.OpenWebhook(messageUpdate(1, 7, "/start"))
	if !ok || id == "" {
		t.Fatalf("OpenWebhook failed: id=%q ok=%v", id, ok)
	}
	chat := int64(7)
	r.RecordCall(&chat, "", okCall("sendMessage"))
	r.CloseWebhook(id, true)

	tr := r.Snapshot()[0]
	if tr.Status != StatusOK || tr.FinishedAt == nil {
		t.Fatalf("webhook trace not closed ok: status=%q finished=%v", tr.Status, tr.FinishedAt)
	}
}

func TestWebhookCloseFailureMarksError(t *testing.T) {
	r := NewRecorder(10, nil)
	id, _ := r.OpenWebhook(messageUpdate(1, 7, "/start"))
	r.CloseWebhook(id, false)

	if got := r.Snapshot()[0].Status; got != StatusError {
		t.Fatalf("status = %q, want error on failed webhook close", got)
	}
}

func TestResetClearsTraces(t *testing.T) {
	r := NewRecorder(10, nil)
	r.OpenForUpdates([]tg.Update{messageUpdate(1, 7, "/start")})
	r.Reset()
	if got := len(r.Snapshot()); got != 0 {
		t.Fatalf("want 0 traces after reset, got %d", got)
	}
}

func TestTrimParamsTruncatesLargeValues(t *testing.T) {
	big := make([]byte, 5000)
	for i := range big {
		big[i] = 'x'
	}
	out := TrimParams(map[string]any{"text": string(big), "n": 5})
	text, _ := out["text"].(string)
	if len(text) <= 4096 || text[len(text)-len("...[truncated]"):] != "...[truncated]" {
		t.Fatalf("large value not truncated, len=%d", len(text))
	}
	if out["n"] != 5 {
		t.Fatalf("small value changed: %v", out["n"])
	}
}
