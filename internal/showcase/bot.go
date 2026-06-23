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

const (
	ReplyPing       = "Ping"
	ReplyButtons    = "Buttons"
	ReplyTraceError = "Trace error"

	CallbackEdit       = "showcase:edit"
	CallbackToast      = "showcase:toast"
	CallbackDeleteTemp = "showcase:delete-temp"
	CallbackReply      = "showcase:reply-keyboard"
	CallbackError      = "showcase:trace-error"
)

type TelegramClient interface {
	SendMessage(params *telego.SendMessageParams) (*telego.Message, error)
	AnswerCallbackQuery(params *telego.AnswerCallbackQueryParams) error
	EditMessageText(params *telego.EditMessageTextParams) (*telego.Message, error)
	DeleteMessage(params *telego.DeleteMessageParams) error
}

type TraceErrorTrigger func(chatID int64) error

type Bot struct {
	client            TelegramClient
	triggerTraceError TraceErrorTrigger
	now               func() time.Time
}

func New(client TelegramClient, trigger TraceErrorTrigger) *Bot {
	if trigger == nil {
		trigger = func(int64) error { return nil }
	}
	return &Bot{
		client:            client,
		triggerTraceError: trigger,
		now:               time.Now,
	}
}

func (b *Bot) Handle(update telego.Update) error {
	switch {
	case update.Message != nil:
		return b.handleMessage(update.Message)
	case update.CallbackQuery != nil:
		return b.handleCallback(update.CallbackQuery)
	default:
		return nil
	}
}

func (b *Bot) handleMessage(message *telego.Message) error {
	text := strings.TrimSpace(message.Text)
	switch text {
	case "/start", ReplyButtons:
		return b.sendStart(message.Chat.ID)
	case ReplyPing:
		_, err := b.client.SendMessage(&telego.SendMessageParams{
			ChatID: telego.ChatID{ID: message.Chat.ID},
			Text:   "Pong. The simulator received your message and the showcase bot answered it.",
		})
		return err
	case ReplyTraceError:
		return b.triggerErrorScenario(message.Chat.ID)
	default:
		if text == "" {
			text = "<empty>"
		}
		_, err := b.client.SendMessage(&telego.SendMessageParams{
			ChatID: telego.ChatID{ID: message.Chat.ID},
			Text:   "Echo: " + text,
		})
		return err
	}
}

func (b *Bot) handleCallback(query *telego.CallbackQuery) error {
	if query.ID == "" {
		return nil
	}

	switch query.Data {
	case CallbackEdit:
		if err := b.client.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Editing the original bot message",
		}); err != nil {
			return err
		}
		msg, ok := accessibleCallbackMessage(query)
		if !ok {
			return b.alert(query.ID, "Callback message is not accessible")
		}
		_, err := b.client.EditMessageText(&telego.EditMessageTextParams{
			ChatID:    telego.ChatID{ID: msg.Chat.ID},
			MessageID: msg.MessageID,
			Text:      "Edited by the showcase bot at " + b.now().Format("15:04:05"),
			ReplyMarkup: &telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{
				{{Text: "Back to buttons", CallbackData: CallbackToast}},
			}},
		})
		return err
	case CallbackToast:
		return b.client.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Toast from answerCallbackQuery",
		})
	case CallbackDeleteTemp:
		if err := b.client.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Creating and deleting a temporary message",
		}); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Cannot resolve chat for temporary message")
		}
		sent, err := b.client.SendMessage(&telego.SendMessageParams{
			ChatID: telego.ChatID{ID: chatID},
			Text:   "Temporary message. It should disappear immediately.",
		})
		if err != nil {
			return err
		}
		return b.client.DeleteMessage(&telego.DeleteMessageParams{
			ChatID:    telego.ChatID{ID: chatID},
			MessageID: sent.MessageID,
		})
	case CallbackReply:
		if err := b.client.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Reply keyboard sent",
		}); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Cannot resolve chat for reply keyboard")
		}
		return b.sendReplyKeyboard(chatID)
	case CallbackError:
		if err := b.client.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Triggering a visible trace error",
			ShowAlert:       true,
		}); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Cannot resolve chat for trace error")
		}
		return b.triggerErrorScenario(chatID)
	default:
		return b.alert(query.ID, "Unknown showcase callback: "+query.Data)
	}
}

func (b *Bot) sendStart(chatID int64) error {
	_, err := b.client.SendMessage(&telego.SendMessageParams{
		ChatID: telego.ChatID{ID: chatID},
		Text: strings.Join([]string{
			"Showcase bot is ready.",
			"Use these buttons to exercise the simulator: callbacks, edits, deletes, reply keyboards, and trace errors.",
		}, "\n"),
		ReplyMarkup: showcaseInlineKeyboard(),
	})
	return err
}

func (b *Bot) sendReplyKeyboard(chatID int64) error {
	_, err := b.client.SendMessage(&telego.SendMessageParams{
		ChatID: telego.ChatID{ID: chatID},
		Text:   "Reply keyboard is active. Press a command below or type any text for echo.",
		ReplyMarkup: &telego.ReplyKeyboardMarkup{
			Keyboard: [][]telego.KeyboardButton{
				{{Text: ReplyPing}, {Text: ReplyButtons}},
				{{Text: ReplyTraceError}},
			},
			ResizeKeyboard:        true,
			InputFieldPlaceholder: "Try Ping, Buttons, or Trace error",
		},
	})
	return err
}

func (b *Bot) triggerErrorScenario(chatID int64) error {
	if err := b.triggerTraceError(chatID); err != nil {
		return err
	}
	_, err := b.client.SendMessage(&telego.SendMessageParams{
		ChatID: telego.ChatID{ID: chatID},
		Text:   "A deliberately invalid Bot API call was sent. The trace panel should show it as an error.",
	})
	return err
}

func (b *Bot) alert(callbackID, text string) error {
	return b.client.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
		CallbackQueryID: callbackID,
		Text:            text,
		ShowAlert:       true,
	})
}

func showcaseInlineKeyboard() *telego.InlineKeyboardMarkup {
	return &telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{
		{
			{Text: "Callback + edit", CallbackData: CallbackEdit},
			{Text: "Toast", CallbackData: CallbackToast},
		},
		{
			{Text: "Delete temp", CallbackData: CallbackDeleteTemp},
			{Text: "Reply keyboard", CallbackData: CallbackReply},
		},
		{
			{Text: "Trace error", CallbackData: CallbackError},
		},
	}}
}

func accessibleCallbackMessage(query *telego.CallbackQuery) (*telego.Message, bool) {
	if query.Message == nil || !query.Message.IsAccessible() {
		return nil, false
	}
	msg, ok := query.Message.(*telego.Message)
	return msg, ok
}

func callbackChatID(query *telego.CallbackQuery) (int64, bool) {
	msg, ok := accessibleCallbackMessage(query)
	if !ok {
		return 0, false
	}
	return msg.Chat.ID, true
}

func NewTraceErrorTrigger(apiBase, token string) TraceErrorTrigger {
	apiBase = strings.TrimRight(apiBase, "/")
	return func(chatID int64) error {
		body, err := json.Marshal(map[string]any{"chat_id": chatID})
		if err != nil {
			return err
		}
		resp, err := http.Post(apiBase+"/bot"+token+"/sendMessage", "application/json", bytes.NewReader(body))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode == 0 {
			return fmt.Errorf("invalid HTTP status from trace error call")
		}
		return nil
	}
}
