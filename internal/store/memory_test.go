package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Zulut30/local-telegram-client/internal/tg"
)

func TestInjectTextDefaultsAndQueues(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	update, err := m.InjectText(ctx, TextInput{Text: "/start"})
	if err != nil {
		t.Fatalf("InjectText: %v", err)
	}
	if update.UpdateID != 1 {
		t.Fatalf("first update id = %d, want 1", update.UpdateID)
	}
	if update.Message == nil || update.Message.Chat.ID != 1 {
		t.Fatalf("chat id should default to 1, got %+v", update.Message)
	}
	if update.Message.From == nil || update.Message.From.FirstName != "Developer" {
		t.Fatalf("from should default, got %+v", update.Message.From)
	}
}

func TestGetUpdatesOffsetSemantics(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := m.InjectText(ctx, TextInput{ChatID: 1, Text: "msg"}); err != nil {
			t.Fatalf("inject: %v", err)
		}
	}

	all, _ := m.GetUpdates(ctx, 0, 100, 0)
	if len(all) != 3 {
		t.Fatalf("offset 0 should return all 3, got %d", len(all))
	}

	tail, _ := m.GetUpdates(ctx, 2, 100, 0)
	if len(tail) != 2 || tail[0].UpdateID != 2 {
		t.Fatalf("offset 2 should return updates 2,3, got %+v", tail)
	}

	drained, _ := m.GetUpdates(ctx, 4, 100, 0)
	if len(drained) != 0 {
		t.Fatalf("offset 4 should drain queue, got %d", len(drained))
	}
}

func TestGetUpdatesLongPollWakesOnInject(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	type result struct {
		updates []tg.Update
		err     error
	}
	done := make(chan result, 1)
	go func() {
		updates, err := m.GetUpdates(ctx, 0, 100, 2*time.Second)
		done <- result{updates, err}
	}()

	// Give the poller a moment to block, then inject.
	time.Sleep(20 * time.Millisecond)
	if _, err := m.InjectText(ctx, TextInput{ChatID: 1, Text: "late"}); err != nil {
		t.Fatalf("inject: %v", err)
	}

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("long poll err: %v", res.err)
		}
		if len(res.updates) != 1 {
			t.Fatalf("long poll should return 1 update, got %d", len(res.updates))
		}
	case <-time.After(time.Second):
		t.Fatal("long poll did not wake on inject")
	}
}

func TestGetUpdatesLongPollTimesOutEmpty(t *testing.T) {
	m := NewMemory()
	start := time.Now()
	updates, err := m.GetUpdates(context.Background(), 0, 100, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(updates) != 0 {
		t.Fatalf("want empty on timeout, got %d", len(updates))
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatal("returned before timeout elapsed")
	}
}

func TestSaveBotMessageResolvesReplyTarget(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	// Seed a user message to reply to.
	upd, _ := m.InjectText(ctx, TextInput{ChatID: 5, Text: "question"})
	target := upd.Message.MessageID

	msg, err := m.SaveBotMessage(ctx, BotMessageInput{
		ChatID:           5,
		Text:             "answer",
		ReplyToMessageID: target,
	})
	if err != nil {
		t.Fatalf("SaveBotMessage: %v", err)
	}
	if msg.ReplyToMessage == nil || msg.ReplyToMessage.MessageID != target {
		t.Fatalf("reply_to_message not resolved: %+v", msg.ReplyToMessage)
	}
	if msg.ReplyToMessage.Text != "question" {
		t.Fatalf("reply_to_message text = %q", msg.ReplyToMessage.Text)
	}
}

func TestSaveBotMessageCarriesThreadAndLinkPreview(t *testing.T) {
	m := NewMemory()
	msg, err := m.SaveBotMessage(context.Background(), BotMessageInput{
		ChatID:               5,
		Text:                 "hello",
		MessageThreadID:      42,
		BusinessConnectionID: "biz_1",
		LinkPreviewOptions:   json.RawMessage(`{"is_disabled":true}`),
	})
	if err != nil {
		t.Fatalf("SaveBotMessage: %v", err)
	}
	if msg.MessageThreadID != 42 || msg.BusinessConnectionID != "biz_1" {
		t.Fatalf("thread/business not carried: %+v", msg)
	}
	if string(msg.LinkPreviewOptions) != `{"is_disabled":true}` {
		t.Fatalf("link preview not carried: %s", msg.LinkPreviewOptions)
	}
}

func TestEditMessageTextPreservesRichMessageWhenAbsent(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	original, _ := m.SaveBotMessage(ctx, BotMessageInput{
		ChatID:      5,
		Text:        "v1",
		RichMessage: json.RawMessage(`{"blocks":["a"]}`),
	})

	edited, err := m.EditMessageText(ctx, EditMessageTextInput{
		ChatID:    5,
		MessageID: original.MessageID,
		Text:      "v2",
	})
	if err != nil {
		t.Fatalf("EditMessageText: %v", err)
	}
	if edited.Text != "v2" {
		t.Fatalf("text not updated: %q", edited.Text)
	}
	if string(edited.RichMessage) != `{"blocks":["a"]}` {
		t.Fatalf("rich message clobbered: %s", edited.RichMessage)
	}
}

func TestEditMessageCaptionUpdatesStoredMessage(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	original, _ := m.SaveBotMessage(ctx, BotMessageInput{ChatID: 5, Caption: "old", MediaKind: "photo"})

	edited, err := m.EditMessageCaption(ctx, EditMessageCaptionInput{
		ChatID:    5,
		MessageID: original.MessageID,
		Caption:   "new",
	})
	if err != nil {
		t.Fatalf("EditMessageCaption: %v", err)
	}
	if edited.Caption != "new" {
		t.Fatalf("caption = %q, want new", edited.Caption)
	}
}

func TestDeleteMessage(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	saved, _ := m.SaveBotMessage(ctx, BotMessageInput{ChatID: 5, Text: "bye"})

	if _, err := m.DeleteMessage(ctx, 5, saved.MessageID); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if _, err := m.DeleteMessage(ctx, 5, saved.MessageID); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("second delete err = %v, want ErrMessageNotFound", err)
	}
}

func TestStateAndReset(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	_, _ = m.InjectText(ctx, TextInput{ChatID: 2, Text: "hi"})
	_, _ = m.SaveBotMessage(ctx, BotMessageInput{ChatID: 2, Text: "yo"})

	state, _ := m.State(ctx)
	if len(state.Chats) != 1 || state.Chats[0].ID != 2 {
		t.Fatalf("state chats = %+v", state.Chats)
	}
	if len(state.Messages["2"]) != 2 {
		t.Fatalf("want 2 messages in chat 2, got %d", len(state.Messages["2"]))
	}

	if err := m.Reset(ctx); err != nil {
		t.Fatalf("reset: %v", err)
	}
	after, _ := m.State(ctx)
	if len(after.Chats) != 0 {
		t.Fatalf("state not cleared after reset: %+v", after.Chats)
	}
}

func TestInjectCallbackRequiresExistingMessage(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	if _, err := m.InjectCallback(ctx, CallbackInput{ChatID: 1, MessageID: 999, Data: "x"}); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("callback on missing message err = %v, want ErrMessageNotFound", err)
	}

	saved, _ := m.SaveBotMessage(ctx, BotMessageInput{ChatID: 1, Text: "menu"})
	upd, err := m.InjectCallback(ctx, CallbackInput{ChatID: 1, MessageID: saved.MessageID, Data: "pick:1"})
	if err != nil {
		t.Fatalf("InjectCallback: %v", err)
	}
	if upd.CallbackQuery == nil || upd.CallbackQuery.Data != "pick:1" {
		t.Fatalf("callback not built: %+v", upd.CallbackQuery)
	}
}
