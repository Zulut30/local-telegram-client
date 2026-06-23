package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/Zulut30/local-telegram-client/internal/tg"
	tracing "github.com/Zulut30/local-telegram-client/internal/trace"
)

type Manager struct {
	mu       sync.RWMutex
	url      string
	lastErr  *DeliveryError
	inFlight int

	client   *http.Client
	logger   *slog.Logger
	recorder *tracing.Recorder
}

type DeliveryError struct {
	At      time.Time
	Message string
}

type Info struct {
	URL                string `json:"url"`
	HasCustomCert      bool   `json:"has_custom_certificate"`
	PendingUpdateCount int    `json:"pending_update_count"`
	LastErrorDate      int64  `json:"last_error_date,omitempty"`
	LastErrorMessage   string `json:"last_error_message,omitempty"`
	MaxConnections     int    `json:"max_connections,omitempty"`
}

func New(logger *slog.Logger, recorder *tracing.Recorder) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger:   logger,
		recorder: recorder,
	}
}

func (m *Manager) Set(url string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.url = url
	m.lastErr = nil
}

func (m *Manager) Delete() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.url = ""
	m.lastErr = nil
	m.inFlight = 0
}

func (m *Manager) Active() bool {
	return m.URL() != ""
}

func (m *Manager) URL() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.url
}

func (m *Manager) Info() Info {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info := Info{
		URL:                m.url,
		HasCustomCert:      false,
		PendingUpdateCount: m.inFlight,
		MaxConnections:     40,
	}
	if m.lastErr != nil {
		info.LastErrorDate = m.lastErr.At.Unix()
		info.LastErrorMessage = m.lastErr.Message
	}
	return info
}

func (m *Manager) Deliver(ctx context.Context, update tg.Update) (bool, error) {
	url := m.URL()
	if url == "" {
		return false, nil
	}

	traceID, hasTrace := "", false
	if m.recorder != nil {
		traceID, hasTrace = m.recorder.OpenWebhook(update)
	}

	m.markInFlight(1)
	defer m.markInFlight(-1)

	body, err := json.Marshal(update)
	if err != nil {
		m.closeTrace(traceID, hasTrace, false)
		m.setError(fmt.Sprintf("marshal webhook update: %v", err))
		return true, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		m.closeTrace(traceID, hasTrace, false)
		m.setError(fmt.Sprintf("create webhook request: %v", err))
		return true, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		m.closeTrace(traceID, hasTrace, false)
		m.setError(err.Error())
		m.logger.Warn("webhook delivery failed", "url", url, "error", err)
		return true, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		err := fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
		m.closeTrace(traceID, hasTrace, false)
		m.setError(err.Error())
		m.logger.Warn("webhook delivery returned non-2xx", "url", url, "status", resp.StatusCode)
		return true, err
	}

	m.closeTrace(traceID, hasTrace, true)
	m.clearError()
	return true, nil
}

func (m *Manager) markInFlight(delta int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.inFlight += delta
	if m.inFlight < 0 {
		m.inFlight = 0
	}
}

func (m *Manager) setError(message string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastErr = &DeliveryError{At: time.Now().UTC(), Message: message}
}

func (m *Manager) clearError() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastErr = nil
}

func (m *Manager) closeTrace(id string, ok, success bool) {
	if !ok || m.recorder == nil {
		return
	}
	m.recorder.CloseWebhook(id, success)
}
