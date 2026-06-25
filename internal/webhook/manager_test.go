package webhook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Zulut30/local-telegram-client/internal/tg"
)

func sampleUpdate() tg.Update {
	return tg.Update{UpdateID: 1, Message: &tg.Message{MessageID: 1, Chat: tg.Chat{ID: 7, Type: "private"}, Text: "/start"}}
}

func TestDeliverNoURLReturnsFalse(t *testing.T) {
	m := New(nil, nil)
	delivered, err := m.Deliver(context.Background(), sampleUpdate())
	if delivered || err != nil {
		t.Fatalf("Deliver with no url = (%v, %v), want (false, nil)", delivered, err)
	}
	if m.Active() {
		t.Fatal("manager should be inactive without a url")
	}
}

func TestSetActivatesAndInfoReports(t *testing.T) {
	m := New(nil, nil)
	m.Set("https://example.test/webhook")
	if !m.Active() {
		t.Fatal("manager should be active after Set")
	}
	if info := m.Info(); info.URL != "https://example.test/webhook" {
		t.Fatalf("info url = %q", info.URL)
	}
	m.Delete()
	if m.Active() {
		t.Fatal("manager should be inactive after Delete")
	}
}

func TestDeliverSuccessClearsError(t *testing.T) {
	var got int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&got, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := New(nil, nil)
	m.Set(srv.URL)
	delivered, err := m.Deliver(context.Background(), sampleUpdate())
	if !delivered || err != nil {
		t.Fatalf("Deliver = (%v, %v), want (true, nil)", delivered, err)
	}
	if atomic.LoadInt64(&got) != 1 {
		t.Fatalf("webhook target hit %d times, want 1", got)
	}
	if msg := m.Info().LastErrorMessage; msg != "" {
		t.Fatalf("last error should be empty on success, got %q", msg)
	}
}

func TestDeliverNon2xxRecordsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	m := New(nil, nil)
	m.Set(srv.URL)
	delivered, err := m.Deliver(context.Background(), sampleUpdate())
	if !delivered || err == nil {
		t.Fatalf("Deliver = (%v, %v), want (true, err)", delivered, err)
	}
	if msg := m.Info().LastErrorMessage; msg == "" {
		t.Fatal("last error should be set after non-2xx response")
	}
}
