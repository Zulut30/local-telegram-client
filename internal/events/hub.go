package events

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Event struct {
	Name string
	Data any
}

type Hub struct {
	register   chan chan Event
	unregister chan chan Event
	broadcast  chan Event
}

func NewHub(bufferSize int) *Hub {
	if bufferSize <= 0 {
		bufferSize = 1000
	}
	h := &Hub{
		register:   make(chan chan Event),
		unregister: make(chan chan Event),
		broadcast:  make(chan Event, bufferSize),
	}
	go h.run()
	return h
}

func (h *Hub) Broadcast(name string, data any) {
	select {
	case h.broadcast <- Event{Name: name, Data: data}:
	default:
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// SSE is a long-lived stream, so the server-wide WriteTimeout would tear it
	// down (and stall on a slow client). Clear the per-connection write deadline.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	ch := make(chan Event, 32)
	h.register <- ch
	defer func() {
		h.unregister <- ch
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSE(w, ev.Name, ev.Data); err != nil {
				return
			}
			flusher.Flush()
		case <-ping.C:
			if err := writeSSE(w, "ping", map[string]any{"ts": time.Now().UTC()}); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (h *Hub) run() {
	subs := map[chan Event]struct{}{}
	for {
		select {
		case ch := <-h.register:
			subs[ch] = struct{}{}
		case ch := <-h.unregister:
			if _, ok := subs[ch]; ok {
				delete(subs, ch)
				close(ch)
			}
		case ev := <-h.broadcast:
			for ch := range subs {
				select {
				case ch <- ev:
				default:
					delete(subs, ch)
					close(ch)
				}
			}
		}
	}
}

func writeSSE(w http.ResponseWriter, name string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", name); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
		return err
	}
	return nil
}
