package tg

import "encoding/json"

type User struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

type Message struct {
	MessageID        int64           `json:"message_id"`
	From             *User           `json:"from,omitempty"`
	Chat             Chat            `json:"chat"`
	Date             int64           `json:"date"`
	Text             string          `json:"text,omitempty"`
	Entities         []MessageEntity `json:"entities,omitempty"`
	ParseMode        string          `json:"parse_mode,omitempty"`
	Caption          string          `json:"caption,omitempty"`
	CaptionEntities  []MessageEntity `json:"caption_entities,omitempty"`
	CaptionParseMode string          `json:"caption_parse_mode,omitempty"`
	Photo            []PhotoSize     `json:"photo,omitempty"`
	PhotoURL         string          `json:"photo_url,omitempty"`
	Document         *FileRef        `json:"document,omitempty"`
	Audio            *FileRef        `json:"audio,omitempty"`
	Video            *FileRef        `json:"video,omitempty"`
	Animation        *FileRef        `json:"animation,omitempty"`
	Voice            *FileRef        `json:"voice,omitempty"`
	VideoNote        *FileRef        `json:"video_note,omitempty"`
	Sticker          *StickerRef     `json:"sticker,omitempty"`
	MediaKind        string          `json:"media_kind,omitempty"`
	MediaURL         string          `json:"media_url,omitempty"`
	RichMessage      json.RawMessage `json:"rich_message,omitempty"`
	ReplyMarkup      json.RawMessage `json:"reply_markup,omitempty"`
}

type PhotoSize struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int    `json:"file_size,omitempty"`
}

type FileRef struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileName     string `json:"file_name,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int    `json:"file_size,omitempty"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
	Duration     int    `json:"duration,omitempty"`
}

type StickerRef struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Type         string `json:"type"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	IsAnimated   bool   `json:"is_animated"`
	IsVideo      bool   `json:"is_video"`
	FileSize     int    `json:"file_size,omitempty"`
}

type MessageEntity struct {
	Type            string `json:"type"`
	Offset          int    `json:"offset"`
	Length          int    `json:"length"`
	URL             string `json:"url,omitempty"`
	User            *User  `json:"user,omitempty"`
	Language        string `json:"language,omitempty"`
	CustomEmojiID   string `json:"custom_emoji_id,omitempty"`
	UnixTime        int64  `json:"unix_time,omitempty"`
	DateTimeFormat  string `json:"date_time_format,omitempty"`
	AlternativeText string `json:"alternative_text,omitempty"`
}

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data,omitempty"`
}
