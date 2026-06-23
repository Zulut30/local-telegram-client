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
	ReplyPing         = "Ping"
	ReplyButtons      = "Buttons"
	ReplyRecipes      = "Recipes"
	ReplyRandomRecipe = "Random recipe"
	ReplyDevTools     = "Dev tools"
	ReplyTraceError   = "Trace error"

	CallbackRecipeList   = "recipe:list"
	CallbackRecipeRandom = "recipe:random"
	CallbackRecipePrefix = "recipe:show:"
	CallbackIngredients  = "recipe:ingredients:"
	CallbackSteps        = "recipe:steps:"
	CallbackDevTools     = "showcase:dev-tools"

	CallbackEdit       = "showcase:edit"
	CallbackToast      = "showcase:toast"
	CallbackDeleteTemp = "showcase:delete-temp"
	CallbackReply      = "showcase:reply-keyboard"
	CallbackError      = "showcase:trace-error"
)

type TelegramClient interface {
	SendMessage(params *telego.SendMessageParams) (*telego.Message, error)
	SendPhoto(params *telego.SendPhotoParams) (*telego.Message, error)
	SendChatAction(params *telego.SendChatActionParams) error
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

type recipe struct {
	ID          string
	Name        string
	Area        string
	Category    string
	Time        string
	Difficulty  string
	PhotoURL    string
	SourceURL   string
	Ingredients []string
	Steps       []string
}

var recipeCatalog = []recipe{
	{
		ID:         "arrabiata",
		Name:       "Spicy Arrabiata Penne",
		Area:       "Italian",
		Category:   "Vegetarian",
		Time:       "25 min",
		Difficulty: "Easy",
		PhotoURL:   "https://www.themealdb.com/images/media/meals/ustsqw1468250014.jpg",
		SourceURL:  "https://www.themealdb.com/meal/52771-spicy-arrabiata-penne-recipe",
		Ingredients: []string{
			"Penne rigate - 1 pound",
			"Olive oil - 1/4 cup",
			"Garlic - 3 cloves",
			"Chopped tomatoes - 1 tin",
			"Red chilli flakes - 1/2 teaspoon",
			"Italian seasoning - 1/2 teaspoon",
			"Basil - 6 leaves",
			"Parmigiano-Reggiano - for serving",
		},
		Steps: []string{
			"Boil pasta in salted water until al dente.",
			"Cook sliced garlic in olive oil until fragrant.",
			"Add tomatoes, chilli flakes, and seasoning; simmer for about 5 minutes.",
			"Toss drained pasta with the sauce, then finish with basil and cheese.",
		},
	},
	{
		ID:         "chicken-handi",
		Name:       "Chicken Handi",
		Area:       "Indian",
		Category:   "Chicken",
		Time:       "50 min",
		Difficulty: "Medium",
		PhotoURL:   "https://www.themealdb.com/images/media/meals/wyxwsp1486979827.jpg",
		SourceURL:  "https://www.themealdb.com/meal/52795-chicken-handi-recipe",
		Ingredients: []string{
			"Chicken - 1.2 kg",
			"Onions - 5 thinly sliced",
			"Tomatoes - 2 finely chopped",
			"Garlic - 8 cloves",
			"Ginger paste - 1 tablespoon",
			"Vegetable oil - 1/4 cup",
			"Cumin seeds - 2 teaspoons",
			"Coriander seeds - 3 teaspoons",
			"Turmeric - 1 teaspoon",
			"Chilli powder - 1 teaspoon",
			"Yogurt - 1 cup",
			"Cream - 3/4 cup",
			"Garam masala - 1 teaspoon",
		},
		Steps: []string{
			"Fry onions until golden, then set them aside.",
			"Cook garlic and tomatoes, then add ginger and ground spices.",
			"Add chicken and cook until sealed and tender.",
			"Lower the heat, stir in yogurt, then finish with cream, fenugreek, and garam masala.",
			"Serve hot with naan or roti.",
		},
	},
	{
		ID:         "beef-pie",
		Name:       "Beef and Mustard Pie",
		Area:       "British",
		Category:   "Beef",
		Time:       "2 h 30 min",
		Difficulty: "Medium",
		PhotoURL:   "https://www.themealdb.com/images/media/meals/sytuqu1511553755.jpg",
		SourceURL:  "https://www.themealdb.com/meal/52874-beef-and-mustard-pie-recipe",
		Ingredients: []string{
			"Beef - 1 kg",
			"Plain flour - 2 tablespoons",
			"Rapeseed oil - 2 tablespoons",
			"Red wine - 200 ml",
			"Beef stock - 400 ml",
			"Onion - 1 sliced",
			"Carrots - 2 chopped",
			"Thyme - 3 sprigs",
			"Mustard - 2 tablespoons",
			"Puff pastry - 400 g",
			"Egg yolks - 2",
			"Green beans - 300 g",
		},
		Steps: []string{
			"Brown floured beef in oil, then build a sauce with wine and stock.",
			"Slow-cook with onion, carrots, thyme, and mustard until tender.",
			"Cool the filling, cover with puff pastry, and brush with egg yolk.",
			"Bake until the pastry is golden, then serve with buttered green beans.",
		},
	},
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
	if len(message.Photo) > 0 {
		return b.handleFoodPhoto(message)
	}

	text := strings.TrimSpace(message.Text)
	switch text {
	case "/start", ReplyButtons, ReplyRecipes:
		return b.sendStart(message.Chat.ID)
	case ReplyRandomRecipe:
		return b.sendRecipePhoto(message.Chat.ID, b.randomRecipe())
	case ReplyDevTools:
		return b.sendDevTools(message.Chat.ID)
	case ReplyPing:
		_, err := b.client.SendMessage(&telego.SendMessageParams{
			ChatID: telego.ChatID{ID: message.Chat.ID},
			Text:   "Pong. The simulator received your message and the recipe bot answered it.",
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
			ReplyMarkup: &telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{
				{{Text: "Open recipes", CallbackData: CallbackRecipeList}},
				{{Text: "Dev tools", CallbackData: CallbackDevTools}},
			}},
		})
		return err
	}
}

func (b *Bot) handleFoodPhoto(message *telego.Message) error {
	text := "Nice photo. I received it as a Telegram photo update."
	if strings.TrimSpace(message.Caption) != "" {
		text += "\nCaption: " + strings.TrimSpace(message.Caption)
	}
	text += "\nPick a recipe idea from the demo catalog:"
	_, err := b.client.SendMessage(&telego.SendMessageParams{
		ChatID:      telego.ChatID{ID: message.Chat.ID},
		Text:        text,
		ReplyMarkup: recipeListKeyboard(),
	})
	return err
}

func (b *Bot) handleCallback(query *telego.CallbackQuery) error {
	if query.ID == "" {
		return nil
	}

	switch {
	case query.Data == CallbackRecipeList:
		if err := b.answer(query.ID, "Opening recipes"); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Cannot resolve chat for recipe list")
		}
		return b.sendRecipeList(chatID)
	case query.Data == CallbackRecipeRandom:
		if err := b.answer(query.ID, "Choosing a recipe"); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Cannot resolve chat for recipe")
		}
		return b.sendRecipePhoto(chatID, b.randomRecipe())
	case strings.HasPrefix(query.Data, CallbackRecipePrefix):
		if err := b.answer(query.ID, "Sending recipe card"); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Cannot resolve chat for recipe")
		}
		found, ok := findRecipe(strings.TrimPrefix(query.Data, CallbackRecipePrefix))
		if !ok {
			return b.alert(query.ID, "Recipe not found")
		}
		return b.sendRecipePhoto(chatID, found)
	case strings.HasPrefix(query.Data, CallbackIngredients):
		if err := b.answer(query.ID, "Opening ingredients"); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Cannot resolve chat for ingredients")
		}
		found, ok := findRecipe(strings.TrimPrefix(query.Data, CallbackIngredients))
		if !ok {
			return b.alert(query.ID, "Recipe not found")
		}
		return b.sendRecipeDetails(chatID, found.Name+" ingredients", found.Ingredients)
	case strings.HasPrefix(query.Data, CallbackSteps):
		if err := b.answer(query.ID, "Opening steps"); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Cannot resolve chat for steps")
		}
		found, ok := findRecipe(strings.TrimPrefix(query.Data, CallbackSteps))
		if !ok {
			return b.alert(query.ID, "Recipe not found")
		}
		return b.sendRecipeDetails(chatID, found.Name+" steps", found.Steps)
	case query.Data == CallbackDevTools:
		if err := b.answer(query.ID, "Opening dev tools"); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Cannot resolve chat for dev tools")
		}
		return b.sendDevTools(chatID)
	case query.Data == CallbackEdit:
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
				{{Text: "Back to dev tools", CallbackData: CallbackDevTools}},
			}},
		})
		return err
	case query.Data == CallbackToast:
		return b.client.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Toast from answerCallbackQuery",
		})
	case query.Data == CallbackDeleteTemp:
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
	case query.Data == CallbackReply:
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
	case query.Data == CallbackError:
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
			"Recipe bot is ready.",
			"Browse recipe cards, open ingredients and steps, attach a food photo, or use dev tools to test callbacks, edits, deletes, reply keyboards, and trace errors.",
		}, "\n"),
		ReplyMarkup: startInlineKeyboard(),
	})
	return err
}

func (b *Bot) sendRecipeList(chatID int64) error {
	_, err := b.client.SendMessage(&telego.SendMessageParams{
		ChatID:      telego.ChatID{ID: chatID},
		Text:        "Choose a recipe from the demo catalog:",
		ReplyMarkup: recipeListKeyboard(),
	})
	return err
}

func (b *Bot) sendRecipePhoto(chatID int64, item recipe) error {
	if err := b.client.SendChatAction(&telego.SendChatActionParams{
		ChatID: telego.ChatID{ID: chatID},
		Action: telego.ChatActionUploadPhoto,
	}); err != nil {
		return err
	}
	_, err := b.client.SendPhoto(&telego.SendPhotoParams{
		ChatID: telego.ChatID{ID: chatID},
		Photo:  telego.InputFile{URL: item.PhotoURL},
		Caption: strings.Join([]string{
			item.Name,
			item.Area + " / " + item.Category,
			"Time: " + item.Time + " | Difficulty: " + item.Difficulty,
		}, "\n"),
		ReplyMarkup: recipeDetailKeyboard(item),
	})
	return err
}

func (b *Bot) sendRecipeDetails(chatID int64, title string, lines []string) error {
	_, err := b.client.SendMessage(&telego.SendMessageParams{
		ChatID: telego.ChatID{ID: chatID},
		Text:   title + "\n" + numberedLines(lines),
		ReplyMarkup: &telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{
			{{Text: "Back to recipes", CallbackData: CallbackRecipeList}},
			{{Text: "Dev tools", CallbackData: CallbackDevTools}},
		}},
	})
	return err
}

func (b *Bot) sendReplyKeyboard(chatID int64) error {
	_, err := b.client.SendMessage(&telego.SendMessageParams{
		ChatID: telego.ChatID{ID: chatID},
		Text:   "Reply keyboard is active. Press a command below or type any text for echo.",
		ReplyMarkup: &telego.ReplyKeyboardMarkup{
			Keyboard: [][]telego.KeyboardButton{
				{{Text: ReplyRecipes}, {Text: ReplyRandomRecipe}},
				{{Text: ReplyPing}, {Text: ReplyDevTools}},
				{{Text: ReplyTraceError}},
			},
			ResizeKeyboard:        true,
			InputFieldPlaceholder: "Try Recipes, Random recipe, or Trace error",
		},
	})
	return err
}

func (b *Bot) sendDevTools(chatID int64) error {
	_, err := b.client.SendMessage(&telego.SendMessageParams{
		ChatID: telego.ChatID{ID: chatID},
		Text: strings.Join([]string{
			"Developer tools are ready.",
			"Use these buttons to exercise Bot API callbacks, edits, deletes, reply keyboards, and trace errors.",
		}, "\n"),
		ReplyMarkup: devToolsInlineKeyboard(),
	})
	return err
}

func (b *Bot) triggerErrorScenario(chatID int64) error {
	if err := b.triggerTraceError(chatID); err != nil {
		return err
	}
	_, err := b.client.SendMessage(&telego.SendMessageParams{
		ChatID: telego.ChatID{ID: chatID},
		Text:   "A deliberately invalid Bot API call was sent. The console should show it as an error.",
	})
	return err
}

func (b *Bot) answer(callbackID, text string) error {
	return b.client.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
		CallbackQueryID: callbackID,
		Text:            text,
	})
}

func (b *Bot) alert(callbackID, text string) error {
	return b.client.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
		CallbackQueryID: callbackID,
		Text:            text,
		ShowAlert:       true,
	})
}

func (b *Bot) randomRecipe() recipe {
	if len(recipeCatalog) == 0 {
		return recipe{}
	}
	return recipeCatalog[int(b.now().UnixNano()%int64(len(recipeCatalog)))]
}

func startInlineKeyboard() *telego.InlineKeyboardMarkup {
	return &telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{
		{
			{Text: "Browse recipes", CallbackData: CallbackRecipeList},
			{Text: "Surprise me", CallbackData: CallbackRecipeRandom},
		},
		{
			{Text: "Reply keyboard", CallbackData: CallbackReply},
			{Text: "Dev tools", CallbackData: CallbackDevTools},
		},
		{
			{Text: "Trace error", CallbackData: CallbackError},
		},
	}}
}

func recipeListKeyboard() *telego.InlineKeyboardMarkup {
	rows := make([][]telego.InlineKeyboardButton, 0, len(recipeCatalog)+2)
	for _, item := range recipeCatalog {
		rows = append(rows, []telego.InlineKeyboardButton{
			{Text: item.Name, CallbackData: CallbackRecipePrefix + item.ID},
		})
	}
	rows = append(rows,
		[]telego.InlineKeyboardButton{{Text: "Surprise me", CallbackData: CallbackRecipeRandom}},
		[]telego.InlineKeyboardButton{{Text: "Dev tools", CallbackData: CallbackDevTools}},
	)
	return &telego.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func recipeDetailKeyboard(item recipe) *telego.InlineKeyboardMarkup {
	return &telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{
		{
			{Text: "Ingredients", CallbackData: CallbackIngredients + item.ID},
			{Text: "Steps", CallbackData: CallbackSteps + item.ID},
		},
		{
			{Text: "Open source", URL: item.SourceURL},
			{Text: "Another", CallbackData: CallbackRecipeRandom},
		},
		{
			{Text: "All recipes", CallbackData: CallbackRecipeList},
		},
	}}
}

func devToolsInlineKeyboard() *telego.InlineKeyboardMarkup {
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
			{Text: "Recipes", CallbackData: CallbackRecipeList},
		},
	}}
}

func findRecipe(id string) (recipe, bool) {
	for _, item := range recipeCatalog {
		if item.ID == id {
			return item, true
		}
	}
	return recipe{}, false
}

func numberedLines(lines []string) string {
	var b strings.Builder
	for index, line := range lines {
		if index > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(fmt.Sprintf("%d. %s", index+1, line))
	}
	return b.String()
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
