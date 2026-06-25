package tg

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageJSONRoundTrip(t *testing.T) {
	msg := Message{
		MessageID:            10,
		MessageThreadID:      42,
		BusinessConnectionID: "biz_1",
		Chat:                 Chat{ID: 7, Type: "private"},
		From:                 &User{ID: 1, IsBot: true, FirstName: "Sim Bot"},
		Date:                 1234567890,
		Text:                 "hello",
		LinkPreviewOptions:   json.RawMessage(`{"is_disabled":true}`),
		ReplyToMessage:       &Message{MessageID: 9, Chat: Chat{ID: 7, Type: "private"}, Text: "question"},
	}

	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var back Message
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.MessageThreadID != 42 || back.BusinessConnectionID != "biz_1" {
		t.Fatalf("thread/business lost: %+v", back)
	}
	if back.ReplyToMessage == nil || back.ReplyToMessage.MessageID != 9 {
		t.Fatalf("reply_to_message lost: %+v", back.ReplyToMessage)
	}
	if string(back.LinkPreviewOptions) != `{"is_disabled":true}` {
		t.Fatalf("link_preview_options lost: %s", back.LinkPreviewOptions)
	}
}

func TestMessageOmitsEmptyOptionalFields(t *testing.T) {
	raw, err := json.Marshal(Message{MessageID: 1, Chat: Chat{ID: 1, Type: "private"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, field := range []string{"reply_to_message", "message_thread_id", "link_preview_options", "business_connection_id", "from"} {
		if strings.Contains(string(raw), field) {
			t.Fatalf("minimal message should omit %q, got %s", field, raw)
		}
	}
}

func TestUpdateCallbackQueryRoundTrip(t *testing.T) {
	update := Update{
		UpdateID: 5,
		CallbackQuery: &CallbackQuery{
			ID:      "cb_5",
			From:    User{ID: 1, FirstName: "Dev"},
			Data:    "pick:1",
			Message: &Message{MessageID: 3, Chat: Chat{ID: 7, Type: "private"}},
		},
	}
	raw, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Update
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.CallbackQuery == nil || back.CallbackQuery.Data != "pick:1" {
		t.Fatalf("callback query lost: %+v", back.CallbackQuery)
	}
	if back.Message != nil {
		t.Fatal("message should be nil for a callback-only update")
	}
}
