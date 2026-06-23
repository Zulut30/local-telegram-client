package showcase

import (
	"errors"
	"strings"
	"testing"

	"github.com/mymmrac/telego"
)

func TestHandleStartSendsInlineKeyboard(t *testing.T) {
	client := &fakeClient{}
	bot := New(client, nil)

	if err := bot.Handle(messageUpdate(42, "/start")); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if len(client.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(client.sent))
	}
	msg := client.sent[0]
	if !strings.Contains(msg.Text, "Бот-рецептов готов") {
		t.Fatalf("start text = %q, want recipe intro", msg.Text)
	}
	markup, ok := msg.ReplyMarkup.(*telego.InlineKeyboardMarkup)
	if !ok {
		t.Fatalf("reply markup = %T, want inline keyboard", msg.ReplyMarkup)
	}
	if got := markup.InlineKeyboard[0][0].CallbackData; got != CallbackRecipeList {
		t.Fatalf("first callback = %q, want %q", got, CallbackRecipeList)
	}
}

func TestHandleTextEchoAndReplyCommands(t *testing.T) {
	client := &fakeClient{}
	triggered := false
	bot := New(client, func(chatID int64) error {
		triggered = chatID == 42
		return nil
	})

	if err := bot.Handle(messageUpdate(42, "hello")); err != nil {
		t.Fatalf("echo returned error: %v", err)
	}
	if got := client.sent[0].Text; got != "Эхо: hello" {
		t.Fatalf("echo text = %q", got)
	}

	if err := bot.Handle(messageUpdate(42, ReplyPing)); err != nil {
		t.Fatalf("ping returned error: %v", err)
	}
	if !strings.Contains(client.sent[1].Text, "Понг") {
		t.Fatalf("ping text = %q, want Понг", client.sent[1].Text)
	}

	if err := bot.Handle(messageUpdate(42, ReplyTraceError)); err != nil {
		t.Fatalf("trace error returned error: %v", err)
	}
	if !triggered {
		t.Fatal("trace error trigger was not called")
	}
}

func TestRecipeCallbackSendsPhoto(t *testing.T) {
	client := &fakeClient{}
	bot := New(client, nil)

	if err := bot.Handle(callbackUpdate(42, 10, "cb_1", CallbackRecipePrefix+"arrabiata")); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if len(client.answered) != 1 {
		t.Fatalf("answered = %d, want 1", len(client.answered))
	}
	if len(client.actions) != 1 || client.actions[0].Action != telego.ChatActionUploadPhoto {
		t.Fatalf("actions = %#v, want upload_photo chat action", client.actions)
	}
	if len(client.photos) != 1 {
		t.Fatalf("photos = %d, want 1", len(client.photos))
	}
	photo := client.photos[0]
	if photo.Photo.URL == "" {
		t.Fatalf("photo URL is empty")
	}
	if !strings.Contains(photo.Caption, "Пенне arrabbiata") {
		t.Fatalf("photo caption = %q, want recipe name", photo.Caption)
	}
	if _, ok := photo.ReplyMarkup.(*telego.InlineKeyboardMarkup); !ok {
		t.Fatalf("reply markup = %T, want inline keyboard", photo.ReplyMarkup)
	}
}

func TestPhotoMessageSuggestsRecipes(t *testing.T) {
	client := &fakeClient{}
	bot := New(client, nil)

	if err := bot.Handle(photoUpdate(42, "lunch")); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if len(client.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(client.sent))
	}
	if !strings.Contains(client.sent[0].Text, "Telegram photo update") {
		t.Fatalf("photo response = %q, want photo acknowledgement", client.sent[0].Text)
	}
	if _, ok := client.sent[0].ReplyMarkup.(*telego.InlineKeyboardMarkup); !ok {
		t.Fatalf("reply markup = %T, want recipe inline keyboard", client.sent[0].ReplyMarkup)
	}
}

func TestHandleCallbacks(t *testing.T) {
	tests := []struct {
		name   string
		data   string
		assert func(*testing.T, *fakeClient)
	}{
		{
			name: "edit",
			data: CallbackEdit,
			assert: func(t *testing.T, client *fakeClient) {
				t.Helper()
				if len(client.answered) != 1 || client.answered[0].CallbackQueryID != "cb_1" {
					t.Fatalf("answered = %#v, want cb_1 answer", client.answered)
				}
				if len(client.edited) != 1 || client.edited[0].MessageID != 10 {
					t.Fatalf("edited = %#v, want message 10 edit", client.edited)
				}
			},
		},
		{
			name: "toast",
			data: CallbackToast,
			assert: func(t *testing.T, client *fakeClient) {
				t.Helper()
				if len(client.answered) != 1 || client.answered[0].Text == "" {
					t.Fatalf("answered = %#v, want toast text", client.answered)
				}
			},
		},
		{
			name: "delete temp",
			data: CallbackDeleteTemp,
			assert: func(t *testing.T, client *fakeClient) {
				t.Helper()
				if len(client.sent) != 1 || len(client.deleted) != 1 {
					t.Fatalf("sent/deleted = %d/%d, want one temporary message deleted", len(client.sent), len(client.deleted))
				}
			},
		},
		{
			name: "reply keyboard",
			data: CallbackReply,
			assert: func(t *testing.T, client *fakeClient) {
				t.Helper()
				if len(client.sent) != 1 {
					t.Fatalf("sent messages = %d, want 1", len(client.sent))
				}
				if _, ok := client.sent[0].ReplyMarkup.(*telego.ReplyKeyboardMarkup); !ok {
					t.Fatalf("reply markup = %T, want reply keyboard", client.sent[0].ReplyMarkup)
				}
			},
		},
		{
			name: "trace error",
			data: CallbackError,
			assert: func(t *testing.T, client *fakeClient) {
				t.Helper()
				if !client.traceTriggered {
					t.Fatal("trace error trigger was not called")
				}
				if len(client.sent) != 1 || !strings.Contains(client.sent[0].Text, "неправильный Bot API вызов") {
					t.Fatalf("sent = %#v, want explanatory message", client.sent)
				}
			},
		},
		{
			name: "rich demo",
			data: CallbackRichDemo,
			assert: func(t *testing.T, client *fakeClient) {
				t.Helper()
				if len(client.actions) != 1 || client.actions[0].Action != telego.ChatActionTyping {
					t.Fatalf("actions = %#v, want typing action", client.actions)
				}
				if len(client.rawCalls) != 1 || client.rawCalls[0].method != "sendRichMessage" {
					t.Fatalf("raw calls = %#v, want sendRichMessage", client.rawCalls)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeClient{}
			bot := New(client, func(chatID int64) error {
				if chatID != 42 {
					return errors.New("unexpected chat id")
				}
				client.traceTriggered = true
				return nil
			})

			if err := bot.Handle(callbackUpdate(42, 10, "cb_1", tt.data)); err != nil {
				t.Fatalf("Handle returned error: %v", err)
			}
			tt.assert(t, client)
		})
	}
}

type fakeClient struct {
	sent           []*telego.SendMessageParams
	photos         []*telego.SendPhotoParams
	actions        []*telego.SendChatActionParams
	answered       []*telego.AnswerCallbackQueryParams
	edited         []*telego.EditMessageTextParams
	deleted        []*telego.DeleteMessageParams
	rawCalls       []fakeRawCall
	nextMessageID  int
	traceTriggered bool
}

type fakeRawCall struct {
	method string
	params any
}

func (f *fakeClient) SendMessage(params *telego.SendMessageParams) (*telego.Message, error) {
	f.nextMessageID++
	f.sent = append(f.sent, params)
	return &telego.Message{
		MessageID: f.nextMessageID,
		Chat:      telego.Chat{ID: params.ChatID.ID, Type: "private"},
		Text:      params.Text,
	}, nil
}

func (f *fakeClient) SendChatAction(params *telego.SendChatActionParams) error {
	f.actions = append(f.actions, params)
	return nil
}

func (f *fakeClient) SendPhoto(params *telego.SendPhotoParams) (*telego.Message, error) {
	f.nextMessageID++
	f.photos = append(f.photos, params)
	return &telego.Message{
		MessageID: f.nextMessageID,
		Chat:      telego.Chat{ID: params.ChatID.ID, Type: "private"},
		Caption:   params.Caption,
		Photo: []telego.PhotoSize{
			{FileID: "photo_id", FileUniqueID: "photo_unique", Width: 640, Height: 480},
		},
	}, nil
}

func (f *fakeClient) AnswerCallbackQuery(params *telego.AnswerCallbackQueryParams) error {
	f.answered = append(f.answered, params)
	return nil
}

func (f *fakeClient) EditMessageText(params *telego.EditMessageTextParams) (*telego.Message, error) {
	f.edited = append(f.edited, params)
	return &telego.Message{
		MessageID: params.MessageID,
		Chat:      telego.Chat{ID: params.ChatID.ID, Type: "private"},
		Text:      params.Text,
	}, nil
}

func (f *fakeClient) DeleteMessage(params *telego.DeleteMessageParams) error {
	f.deleted = append(f.deleted, params)
	return nil
}

func (f *fakeClient) Call(method string, params any, _ any) error {
	f.rawCalls = append(f.rawCalls, fakeRawCall{method: method, params: params})
	return nil
}

func messageUpdate(chatID int64, text string) telego.Update {
	return telego.Update{
		UpdateID: 1,
		Message: &telego.Message{
			MessageID: 1,
			Chat:      telego.Chat{ID: chatID, Type: "private"},
			Text:      text,
		},
	}
}

func photoUpdate(chatID int64, caption string) telego.Update {
	return telego.Update{
		UpdateID: 1,
		Message: &telego.Message{
			MessageID: 1,
			Chat:      telego.Chat{ID: chatID, Type: "private"},
			Caption:   caption,
			Photo: []telego.PhotoSize{
				{FileID: "photo_id", FileUniqueID: "photo_unique", Width: 640, Height: 480},
			},
		},
	}
}

func callbackUpdate(chatID int64, messageID int, callbackID, data string) telego.Update {
	return telego.Update{
		UpdateID: 2,
		CallbackQuery: &telego.CallbackQuery{
			ID: callbackID,
			Message: &telego.Message{
				MessageID: messageID,
				Chat:      telego.Chat{ID: chatID, Type: "private"},
				Date:      1,
			},
			Data: data,
		},
	}
}
