package showcase

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mymmrac/telego"
)

type APIClient struct {
	baseURL string
	token   string
	client  *http.Client
}

type apiEnvelope struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result,omitempty"`
	ErrorCode   int             `json:"error_code,omitempty"`
	Description string          `json:"description,omitempty"`
}

type APIError struct {
	Method      string
	HTTPStatus  int
	ErrorCode   int
	Description string
}

func (e *APIError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("%s failed: HTTP %d, error %d: %s", e.Method, e.HTTPStatus, e.ErrorCode, e.Description)
	}
	return fmt.Sprintf("%s failed: HTTP %d", e.Method, e.HTTPStatus)
}

func NewAPIClient(baseURL, token string) *APIClient {
	return &APIClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *APIClient) GetUpdates(params *telego.GetUpdatesParams) ([]telego.Update, error) {
	var updates []telego.Update
	if err := c.call("getUpdates", params, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (c *APIClient) SetWebhook(params *telego.SetWebhookParams) error {
	return c.call("setWebhook", params, nil)
}

func (c *APIClient) DeleteWebhook(params *telego.DeleteWebhookParams) error {
	return c.call("deleteWebhook", params, nil)
}

func (c *APIClient) SendMessage(params *telego.SendMessageParams) (*telego.Message, error) {
	var message telego.Message
	if err := c.call("sendMessage", params, &message); err != nil {
		return nil, err
	}
	return &message, nil
}

func (c *APIClient) SendPhoto(params *telego.SendPhotoParams) (*telego.Message, error) {
	var message telego.Message
	if err := c.call("sendPhoto", params, &message); err != nil {
		return nil, err
	}
	return &message, nil
}

func (c *APIClient) SendChatAction(params *telego.SendChatActionParams) error {
	return c.call("sendChatAction", params, nil)
}

func (c *APIClient) AnswerCallbackQuery(params *telego.AnswerCallbackQueryParams) error {
	return c.call("answerCallbackQuery", params, nil)
}

func (c *APIClient) EditMessageText(params *telego.EditMessageTextParams) (*telego.Message, error) {
	var message telego.Message
	if err := c.call("editMessageText", params, &message); err != nil {
		return nil, err
	}
	return &message, nil
}

func (c *APIClient) DeleteMessage(params *telego.DeleteMessageParams) error {
	return c.call("deleteMessage", params, nil)
}

func (c *APIClient) Call(method string, params any, result any) error {
	return c.call(method, params, result)
}

func (c *APIClient) call(method string, params any, result any) error {
	body := []byte("{}")
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal %s params: %w", method, err)
		}
		body = raw
	}

	resp, err := c.client.Post(c.baseURL+"/bot"+c.token+"/"+method, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%s request: %w", method, err)
	}
	defer resp.Body.Close()

	var envelope apiEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	if resp.StatusCode >= http.StatusBadRequest || !envelope.OK {
		return &APIError{
			Method:      method,
			HTTPStatus:  resp.StatusCode,
			ErrorCode:   envelope.ErrorCode,
			Description: envelope.Description,
		}
	}
	if result == nil || len(envelope.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(envelope.Result, result); err != nil {
		return fmt.Errorf("decode %s result: %w", method, err)
	}
	return nil
}
