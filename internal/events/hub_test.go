package events

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// readSSE consumes an SSE stream and reports the first "event: <name>" line and
// the "data:" line that follows it, or fails the test on timeout.
func readSSE(t *testing.T, resp *http.Response, wantEvent string) string {
	t.Helper()
	lines := make(chan string, 64)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case line, ok := <-lines:
			if !ok {
				t.Fatal("stream closed before event arrived")
			}
			if line == "event: "+wantEvent {
				// The data line is next.
				select {
				case data := <-lines:
					return data
				case <-deadline:
					t.Fatal("timed out waiting for data line")
				}
			}
		case <-deadline:
			t.Fatalf("timed out waiting for event %q", wantEvent)
		}
	}
}

func TestHubDeliversBroadcastToSubscriber(t *testing.T) {
	hub := NewHub(16)
	srv := httptest.NewServer(hub)
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type = %q", ct)
	}

	// Registration completes before headers are flushed, so one broadcast suffices;
	// broadcast a couple of times to be robust against scheduling.
	go func() {
		for i := 0; i < 5; i++ {
			hub.Broadcast("message", map[string]any{"op": "created", "n": i})
			time.Sleep(10 * time.Millisecond)
		}
	}()

	data := readSSE(t, resp, "message")
	if !strings.HasPrefix(data, "data: ") || !strings.Contains(data, `"op":"created"`) {
		t.Fatalf("unexpected data line: %q", data)
	}
}

func TestHubFanOutToMultipleSubscribers(t *testing.T) {
	hub := NewHub(16)
	srv := httptest.NewServer(hub)
	defer srv.Close()

	resp1, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("subscribe 1: %v", err)
	}
	defer resp1.Body.Close()
	resp2, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("subscribe 2: %v", err)
	}
	defer resp2.Body.Close()

	go func() {
		for i := 0; i < 5; i++ {
			hub.Broadcast("trace", map[string]any{"op": "open"})
			time.Sleep(10 * time.Millisecond)
		}
	}()

	if d := readSSE(t, resp1, "trace"); !strings.Contains(d, `"op":"open"`) {
		t.Fatalf("subscriber 1 bad data: %q", d)
	}
	if d := readSSE(t, resp2, "trace"); !strings.Contains(d, `"op":"open"`) {
		t.Fatalf("subscriber 2 bad data: %q", d)
	}
}

func TestBroadcastNeverBlocksWithoutSubscribers(t *testing.T) {
	hub := NewHub(1)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			hub.Broadcast("noise", i)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Broadcast blocked with no subscribers")
	}
}
