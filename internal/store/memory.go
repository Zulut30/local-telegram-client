package store

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Zulut30/local-telegram-client/internal/tg"
)

type Store interface {
	InjectText(ctx context.Context, input TextInput) (tg.Update, error)
	GetUpdates(ctx context.Context, offset int64, limit int, timeout time.Duration) ([]tg.Update, error)
	SaveBotMessage(ctx context.Context, input BotMessageInput) (tg.Message, error)
	State(ctx context.Context) (State, error)
	Reset(ctx context.Context) error
}

type TextInput struct {
	ChatID    int64
	UserID    int64
	Username  string
	FirstName string
	Text      string
}

type BotMessageInput struct {
	From             tg.User
	ChatID           int64
	Text             string
	ReplyMarkup      json.RawMessage
	ReplyToMessageID int64
}

type State struct {
	Chats    []tg.Chat               `json:"chats"`
	Messages map[string][]tg.Message `json:"messages"`
}

type Memory struct {
	mu            sync.Mutex
	notify        chan struct{}
	nextUpdateID  int64
	nextMessageID int64
	updates       []tg.Update
	chats         map[int64]tg.Chat
	messages      map[int64][]tg.Message
}

func NewMemory() *Memory {
	return &Memory{
		notify:        make(chan struct{}),
		nextUpdateID:  1,
		nextMessageID: 1,
		chats:         make(map[int64]tg.Chat),
		messages:      make(map[int64][]tg.Message),
	}
}

func (m *Memory) InjectText(_ context.Context, input TextInput) (tg.Update, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if input.ChatID == 0 {
		input.ChatID = 1
	}
	if input.UserID == 0 {
		input.UserID = input.ChatID
	}
	if input.FirstName == "" {
		input.FirstName = "Developer"
	}

	chat := m.chatLocked(input.ChatID, input.Username, input.FirstName)
	user := tg.User{
		ID:        input.UserID,
		IsBot:     false,
		FirstName: input.FirstName,
		Username:  input.Username,
	}
	msg := tg.Message{
		MessageID: m.nextMessageID,
		From:      &user,
		Chat:      chat,
		Date:      time.Now().Unix(),
		Text:      input.Text,
	}
	m.nextMessageID++
	m.messages[chat.ID] = append(m.messages[chat.ID], msg)

	update := tg.Update{
		UpdateID: m.nextUpdateID,
		Message:  &msg,
	}
	m.nextUpdateID++
	m.updates = append(m.updates, update)
	m.signalLocked()

	return update, nil
}

func (m *Memory) GetUpdates(ctx context.Context, offset int64, limit int, timeout time.Duration) ([]tg.Update, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	for {
		m.mu.Lock()
		if offset > 0 {
			m.dropBeforeLocked(offset)
		}
		updates := m.pendingLocked(offset, limit)
		if len(updates) > 0 || timeout == 0 {
			m.mu.Unlock()
			return updates, nil
		}
		notify := m.notify
		m.mu.Unlock()

		select {
		case <-ctx.Done():
			return []tg.Update{}, nil
		case <-notify:
		}
	}
}

func (m *Memory) SaveBotMessage(_ context.Context, input BotMessageInput) (tg.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	chat := m.chatLocked(input.ChatID, "", "")
	msg := tg.Message{
		MessageID:   m.nextMessageID,
		From:        &input.From,
		Chat:        chat,
		Date:        time.Now().Unix(),
		Text:        input.Text,
		ReplyMarkup: input.ReplyMarkup,
	}
	m.nextMessageID++
	m.messages[chat.ID] = append(m.messages[chat.ID], msg)
	m.signalLocked()

	return msg, nil
}

func (m *Memory) State(_ context.Context) (State, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	chats := make([]tg.Chat, 0, len(m.chats))
	for _, chat := range m.chats {
		chats = append(chats, chat)
	}
	sort.Slice(chats, func(i, j int) bool {
		return chats[i].ID < chats[j].ID
	})

	messages := make(map[string][]tg.Message, len(m.messages))
	for chatID, entries := range m.messages {
		copied := make([]tg.Message, len(entries))
		copy(copied, entries)
		messages[strconv.FormatInt(chatID, 10)] = copied
	}

	return State{Chats: chats, Messages: messages}, nil
}

func (m *Memory) Reset(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.nextUpdateID = 1
	m.nextMessageID = 1
	m.updates = nil
	m.chats = make(map[int64]tg.Chat)
	m.messages = make(map[int64][]tg.Message)
	m.signalLocked()
	return nil
}

func (m *Memory) chatLocked(chatID int64, username, firstName string) tg.Chat {
	if chat, ok := m.chats[chatID]; ok {
		return chat
	}

	chat := tg.Chat{
		ID:        chatID,
		Type:      "private",
		Username:  username,
		FirstName: firstName,
	}
	m.chats[chatID] = chat
	return chat
}

func (m *Memory) dropBeforeLocked(offset int64) {
	idx := 0
	for idx < len(m.updates) && m.updates[idx].UpdateID < offset {
		idx++
	}
	if idx > 0 {
		m.updates = append([]tg.Update(nil), m.updates[idx:]...)
	}
}

func (m *Memory) pendingLocked(offset int64, limit int) []tg.Update {
	out := make([]tg.Update, 0, limit)
	for _, update := range m.updates {
		if offset == 0 || update.UpdateID >= offset {
			out = append(out, update)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}

func (m *Memory) signalLocked() {
	close(m.notify)
	m.notify = make(chan struct{})
}
