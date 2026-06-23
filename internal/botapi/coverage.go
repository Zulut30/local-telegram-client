package botapi

const (
	BotAPIVersion     = "10.1"
	BotAPIGeneratedAt = "2026-06-23"
	BotAPISourceURL   = "https://core.telegram.org/bots/api"

	CoverageStateful          = "stateful"
	CoverageUIRendered        = "ui_rendered"
	CoverageCompatibilityStub = "compatibility_stub"
	CoverageNotYetSemantic    = "not_yet_semantic"
)

type CoverageReport struct {
	APIVersion  string           `json:"api_version"`
	APIMode     string           `json:"api_mode"`
	GeneratedAt string           `json:"generated_at"`
	SourceURL   string           `json:"source_url"`
	Summary     CoverageSummary  `json:"summary"`
	Methods     []CoverageMethod `json:"methods"`
}

type CoverageSummary struct {
	Total             int `json:"total"`
	Stateful          int `json:"stateful"`
	UIRendered        int `json:"ui_rendered"`
	CompatibilityStub int `json:"compatibility_stub"`
	NotYetSemantic    int `json:"not_yet_semantic"`
}

type CoverageMethod struct {
	Name  string `json:"name"`
	Level string `json:"level"`
	Notes string `json:"notes"`
}

func Coverage(apiMode string) CoverageReport {
	methods := make([]CoverageMethod, 0, len(botAPIMethodNames))
	summary := CoverageSummary{Total: len(botAPIMethodNames)}
	for _, name := range botAPIMethodNames {
		method := coverageForMethod(name)
		methods = append(methods, method)
		switch method.Level {
		case CoverageStateful:
			summary.Stateful++
		case CoverageUIRendered:
			summary.UIRendered++
		case CoverageNotYetSemantic:
			summary.NotYetSemantic++
		default:
			summary.CompatibilityStub++
		}
	}
	return CoverageReport{
		APIVersion:  BotAPIVersion,
		APIMode:     apiMode,
		GeneratedAt: BotAPIGeneratedAt,
		SourceURL:   BotAPISourceURL,
		Summary:     summary,
		Methods:     methods,
	}
}

func MethodCoverage(method string) (CoverageMethod, bool) {
	canonical, ok := canonicalBotAPIMethod(method)
	if !ok {
		return CoverageMethod{}, false
	}
	return coverageForMethod(canonical), true
}

func StrictSupports(method string) bool {
	coverage, ok := MethodCoverage(method)
	if !ok {
		return false
	}
	return coverage.Level == CoverageStateful || coverage.Level == CoverageUIRendered
}

func strictUnsupportedDescription(method string) string {
	coverage, ok := MethodCoverage(method)
	if !ok {
		return "method not found"
	}
	return method + " is " + coverage.Level + " and is disabled in strict api mode"
}

func coverageForMethod(method string) CoverageMethod {
	if note, ok := statefulCoverageNotes[method]; ok {
		return CoverageMethod{Name: method, Level: CoverageStateful, Notes: note}
	}
	if note, ok := notYetSemanticCoverageNotes[method]; ok {
		return CoverageMethod{Name: method, Level: CoverageNotYetSemantic, Notes: note}
	}
	if isGenericSendMessageMethod(method) {
		return CoverageMethod{
			Name:  method,
			Level: CoverageUIRendered,
			Notes: "creates a simplified visible message for local UI testing; full Telegram validation is not modeled yet",
		}
	}
	return CoverageMethod{
		Name:  method,
		Level: CoverageCompatibilityStub,
		Notes: "recognized in compat mode and returns deterministic placeholder data",
	}
}

var statefulCoverageNotes = map[string]string{
	"getMe":                  "returns the simulator bot identity derived from the configured token",
	"getUpdates":             "reads and acknowledges the in-memory polling queue",
	"setWebhook":             "stores a local webhook target and optionally drops pending updates",
	"deleteWebhook":          "clears the local webhook target and optionally drops pending updates",
	"getWebhookInfo":         "reports local webhook URL, pending count, and last delivery error",
	"sendMessage":            "stores a bot text message, reply markup, entities, and broadcasts it to the UI",
	"sendPhoto":              "stores a bot photo-like message with caption, markup, and UI media preview",
	"sendRichMessage":        "stores a rich-message payload rendered by the browser UI",
	"sendMessageDraft":       "broadcasts a temporary typing draft event to the browser UI",
	"sendRichMessageDraft":   "broadcasts a temporary rich-message draft event to the browser UI",
	"sendChatAction":         "broadcasts a temporary chat action such as typing or upload_photo",
	"sendMediaGroup":         "stores a simplified group of media-like bot messages",
	"copyMessage":            "creates a simplified copied message and returns its message_id",
	"editMessageText":        "edits stored bot messages and broadcasts an edit event",
	"editMessageReplyMarkup": "edits stored reply markup and broadcasts an edit event",
	"deleteMessage":          "deletes a stored message and broadcasts a delete event",
	"deleteMessages":         "deletes stored messages and broadcasts delete events for found messages",
	"answerCallbackQuery":    "broadcasts local callback toast or alert events to the UI",
	"getCustomEmojiStickers": "returns deterministic custom emoji sticker metadata for UI rendering",
}

var notYetSemanticCoverageNotes = map[string]string{
	"getFile":                           "file metadata is stubbed; durable file storage and download bytes are not implemented yet",
	"uploadStickerFile":                 "multipart file storage is not implemented yet",
	"sendPaidMedia":                     "paid media and Stars payment semantics are not implemented yet",
	"sendInvoice":                       "invoice, shipping, pre-checkout, and payment update lifecycle is not implemented yet",
	"createInvoiceLink":                 "invoice links are placeholders until payments become stateful",
	"answerShippingQuery":               "shipping query lifecycle is not implemented yet",
	"answerPreCheckoutQuery":            "pre-checkout query lifecycle is not implemented yet",
	"getMyStarBalance":                  "Telegram Stars balance is placeholder data",
	"getStarTransactions":               "Telegram Stars transaction history is placeholder data",
	"refundStarPayment":                 "Telegram Stars refunds are not stateful yet",
	"editUserStarSubscription":          "Telegram Stars subscriptions are not stateful yet",
	"answerWebAppQuery":                 "Mini App result flow is not stateful yet",
	"savePreparedInlineMessage":         "prepared inline messages are placeholders",
	"savePreparedKeyboardButton":        "prepared keyboard buttons are placeholders",
	"answerInlineQuery":                 "inline query injection and selected-result lifecycle are not implemented yet",
	"sendChatJoinRequestWebApp":         "chat join request Mini App flow is not implemented yet",
	"getBusinessConnection":             "business account state is placeholder data",
	"getManagedBotToken":                "managed bot token flow is placeholder data",
	"replaceManagedBotToken":            "managed bot token flow is placeholder data",
	"getManagedBotAccessSettings":       "managed bot access settings are placeholder data",
	"setManagedBotAccessSettings":       "managed bot access settings are not stateful yet",
	"readBusinessMessage":               "business message state is not implemented yet",
	"deleteBusinessMessages":            "business message state is not implemented yet",
	"setBusinessAccountName":            "business account state is not implemented yet",
	"setBusinessAccountUsername":        "business account state is not implemented yet",
	"setBusinessAccountBio":             "business account state is not implemented yet",
	"setBusinessAccountProfilePhoto":    "business account media storage is not implemented yet",
	"removeBusinessAccountProfilePhoto": "business account media storage is not implemented yet",
	"setBusinessAccountGiftSettings":    "business gift settings are not implemented yet",
	"getBusinessAccountStarBalance":     "business Stars balance is placeholder data",
	"transferBusinessAccountStars":      "business Stars transfers are not stateful yet",
	"getBusinessAccountGifts":           "business gifts are placeholder data",
	"getUserGifts":                      "user gifts are placeholder data",
	"getChatGifts":                      "chat gifts are placeholder data",
	"getAvailableGifts":                 "available gifts are placeholder data",
	"sendGift":                          "gift delivery is not stateful yet",
	"giftPremiumSubscription":           "premium gift subscription lifecycle is not implemented yet",
	"convertGiftToStars":                "gift conversion is not stateful yet",
	"upgradeGift":                       "gift upgrade is not stateful yet",
	"transferGift":                      "gift transfer is not stateful yet",
	"postStory":                         "story lifecycle is not implemented yet",
	"repostStory":                       "story lifecycle is not implemented yet",
	"editStory":                         "story lifecycle is not implemented yet",
	"deleteStory":                       "story lifecycle is not implemented yet",
	"createForumTopic":                  "forum topic state is not implemented yet",
	"editForumTopic":                    "forum topic state is not implemented yet",
	"closeForumTopic":                   "forum topic state is not implemented yet",
	"reopenForumTopic":                  "forum topic state is not implemented yet",
	"deleteForumTopic":                  "forum topic state is not implemented yet",
	"unpinAllForumTopicMessages":        "forum topic pin state is not implemented yet",
	"editGeneralForumTopic":             "general forum topic state is not implemented yet",
	"closeGeneralForumTopic":            "general forum topic state is not implemented yet",
	"reopenGeneralForumTopic":           "general forum topic state is not implemented yet",
	"hideGeneralForumTopic":             "general forum topic state is not implemented yet",
	"unhideGeneralForumTopic":           "general forum topic state is not implemented yet",
	"unpinAllGeneralForumTopicMessages": "general forum topic pin state is not implemented yet",
	"sendGame":                          "game messages and score lifecycle are not implemented yet",
	"setGameScore":                      "game score lifecycle is not implemented yet",
	"getGameHighScores":                 "game score lifecycle is not implemented yet",
	"setPassportDataErrors":             "passport flow is not implemented yet",
}
