package trace

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/Zulut30/local-telegram-client/internal/events"
	"github.com/Zulut30/local-telegram-client/internal/tg"
)

const (
	StatusOpen  = "open"
	StatusOK    = "ok"
	StatusError = "error"

	CorrelationInferred = "inferred"
)

type Trace struct {
	ID          string         `json:"id"`
	Inbound     *InboundEvent  `json:"inbound,omitempty"`
	Calls       []OutboundCall `json:"calls"`
	StartedAt   time.Time      `json:"started_at"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
	Status      string         `json:"status"`
	Correlation string         `json:"correlation"`
	Orphan      bool           `json:"orphan,omitempty"`
}

type InboundEvent struct {
	UpdateID int64     `json:"update_id"`
	Type     string    `json:"type"`
	ChatID   int64     `json:"chat_id"`
	Text     string    `json:"text,omitempty"`
	At       time.Time `json:"at"`
}

type OutboundCall struct {
	Method      string         `json:"method"`
	Params      map[string]any `json:"params,omitempty"`
	HTTPStatus  int            `json:"http_status"`
	OK          bool           `json:"ok"`
	ErrorCode   int            `json:"error_code,omitempty"`
	ErrorDesc   string         `json:"error_desc,omitempty"`
	LatencyMS   int64          `json:"latency_ms"`
	At          time.Time      `json:"at"`
	Correlation string         `json:"correlation"`
}

type EventPayload struct {
	Op    string `json:"op"`
	Trace Trace  `json:"trace"`
}

type Recorder struct {
	mu           sync.Mutex
	hub          *events.Hub
	limit        int
	windowTTL    time.Duration
	nextID       int64
	ring         []Trace
	traces       map[string]*Trace
	activeByChat map[int64]string
	timers       map[string]*time.Timer
}

func NewRecorder(limit int, hub *events.Hub) *Recorder {
	if limit <= 0 {
		limit = 1000
	}
	return &Recorder{
		hub:          hub,
		limit:        limit,
		windowTTL:    5 * time.Second,
		nextID:       1,
		traces:       make(map[string]*Trace),
		activeByChat: make(map[int64]string),
		timers:       make(map[string]*time.Timer),
	}
}

func (r *Recorder) OpenForUpdates(updates []tg.Update) {
	for _, update := range updates {
		inbound, ok := inboundFromUpdate(update)
		if !ok {
			continue
		}
		r.open(inbound)
	}
}

func (r *Recorder) FlushOpen() {
	r.mu.Lock()
	ids := make([]string, 0, len(r.activeByChat))
	for _, id := range r.activeByChat {
		ids = append(ids, id)
	}
	r.mu.Unlock()

	for _, id := range ids {
		r.close(id)
	}
}

func (r *Recorder) RecordCall(chatID *int64, call OutboundCall) {
	call.At = time.Now().UTC()
	call.Correlation = CorrelationInferred
	call.Params = TrimParams(call.Params)

	r.mu.Lock()
	if chatID != nil {
		if id, ok := r.activeByChat[*chatID]; ok {
			trace := r.traces[id]
			trace.Calls = append(trace.Calls, call)
			if !call.OK {
				trace.Status = StatusError
			}
			snapshot := cloneTrace(*trace)
			r.upsertLocked(*trace)
			r.mu.Unlock()
			r.broadcast("update", snapshot)
			return
		}
	}

	trace := r.newTraceLocked(nil, true)
	trace.Calls = append(trace.Calls, call)
	if call.OK {
		trace.Status = StatusOK
	} else {
		trace.Status = StatusError
	}
	now := time.Now().UTC()
	trace.FinishedAt = &now
	r.upsertLocked(*trace)
	snapshot := cloneTrace(*trace)
	r.mu.Unlock()

	r.broadcast("open", snapshot)
	r.broadcast("close", snapshot)
}

func (r *Recorder) Snapshot() []Trace {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]Trace, len(r.ring))
	for i, trace := range r.ring {
		out[i] = cloneTrace(trace)
	}
	return out
}

func (r *Recorder) open(inbound InboundEvent) {
	r.mu.Lock()
	trace := r.newTraceLocked(&inbound, false)
	r.activeByChat[inbound.ChatID] = trace.ID
	r.upsertLocked(*trace)
	snapshot := cloneTrace(*trace)
	timer := time.AfterFunc(r.windowTTL, func() {
		r.close(snapshot.ID)
	})
	r.timers[trace.ID] = timer
	r.mu.Unlock()

	r.broadcast("open", snapshot)
}

func (r *Recorder) close(id string) {
	r.mu.Lock()
	trace, ok := r.traces[id]
	if !ok || trace.Status == StatusOK || trace.FinishedAt != nil {
		r.mu.Unlock()
		return
	}
	if trace.Status == StatusOpen {
		trace.Status = StatusOK
	}
	now := time.Now().UTC()
	trace.FinishedAt = &now
	if trace.Inbound != nil {
		delete(r.activeByChat, trace.Inbound.ChatID)
	}
	if timer := r.timers[id]; timer != nil {
		timer.Stop()
		delete(r.timers, id)
	}
	r.upsertLocked(*trace)
	snapshot := cloneTrace(*trace)
	r.mu.Unlock()

	r.broadcast("close", snapshot)
}

func (r *Recorder) newTraceLocked(inbound *InboundEvent, orphan bool) *Trace {
	id := "tr_" + strconv.FormatInt(r.nextID, 10)
	r.nextID++
	trace := &Trace{
		ID:          id,
		Inbound:     inbound,
		Calls:       []OutboundCall{},
		StartedAt:   time.Now().UTC(),
		Status:      StatusOpen,
		Correlation: CorrelationInferred,
		Orphan:      orphan,
	}
	r.traces[id] = trace
	return trace
}

func (r *Recorder) upsertLocked(trace Trace) {
	for i := range r.ring {
		if r.ring[i].ID == trace.ID {
			r.ring[i] = cloneTrace(trace)
			return
		}
	}
	if len(r.ring) == r.limit {
		delete(r.traces, r.ring[0].ID)
		r.ring = append(r.ring[:0], r.ring[1:]...)
	}
	r.ring = append(r.ring, cloneTrace(trace))
}

func (r *Recorder) broadcast(op string, trace Trace) {
	if r.hub == nil {
		return
	}
	r.hub.Broadcast("trace", EventPayload{Op: op, Trace: trace})
}

func inboundFromUpdate(update tg.Update) (InboundEvent, bool) {
	if update.Message != nil {
		return InboundEvent{
			UpdateID: update.UpdateID,
			Type:     "message",
			ChatID:   update.Message.Chat.ID,
			Text:     update.Message.Text,
			At:       time.Now().UTC(),
		}, true
	}
	if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {
		return InboundEvent{
			UpdateID: update.UpdateID,
			Type:     "callback_query",
			ChatID:   update.CallbackQuery.Message.Chat.ID,
			Text:     update.CallbackQuery.Data,
			At:       time.Now().UTC(),
		}, true
	}
	return InboundEvent{}, false
}

func TrimParams(params map[string]any) map[string]any {
	if params == nil {
		return nil
	}
	out := make(map[string]any, len(params))
	for key, value := range params {
		out[key] = trimValue(value)
	}
	return out
}

func trimValue(value any) any {
	const max = 4096
	text := fmt.Sprint(value)
	if len(text) <= max {
		return value
	}
	return text[:max] + "...[truncated]"
}

func cloneTrace(trace Trace) Trace {
	if trace.Inbound != nil {
		inbound := *trace.Inbound
		trace.Inbound = &inbound
	}
	if trace.FinishedAt != nil {
		finished := *trace.FinishedAt
		trace.FinishedAt = &finished
	}
	if trace.Calls == nil {
		trace.Calls = []OutboundCall{}
	} else {
		trace.Calls = append([]OutboundCall(nil), trace.Calls...)
	}
	for i := range trace.Calls {
		trace.Calls[i].Params = TrimParams(trace.Calls[i].Params)
	}
	return trace
}
