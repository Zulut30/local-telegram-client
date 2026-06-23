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
	ReplyPing         = "Пинг"
	ReplyButtons      = "Кнопки"
	ReplyRecipes      = "Рецепты"
	ReplyRandomRecipe = "Случайный рецепт"
	ReplyDevTools     = "Инструменты"
	ReplyTraceError   = "Ошибка trace"

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
		Name:       "Пенне arrabbiata",
		Area:       "Италия",
		Category:   "Вегетарианское",
		Time:       "25 мин",
		Difficulty: "легко",
		PhotoURL:   "https://www.themealdb.com/images/media/meals/ustsqw1468250014.jpg",
		SourceURL:  "https://www.themealdb.com/meal/52771-spicy-arrabiata-penne-recipe",
		Ingredients: []string{
			"Пенне ригате - 450 г",
			"Оливковое масло - 1/4 стакана",
			"Чеснок - 3 зубчика",
			"Рубленые томаты - 1 банка",
			"Хлопья чили - 1/2 ч. л.",
			"Итальянские травы - 1/2 ч. л.",
			"Базилик - 6 листьев",
			"Пармиджано-реджано - для подачи",
		},
		Steps: []string{
			"Отварите пасту в соленой воде до al dente.",
			"Прогрейте нарезанный чеснок в оливковом масле до аромата.",
			"Добавьте томаты, чили и травы; тушите около 5 минут.",
			"Смешайте пасту с соусом, затем добавьте базилик и сыр.",
		},
	},
	{
		ID:         "chicken-handi",
		Name:       "Курица handi",
		Area:       "Индия",
		Category:   "Курица",
		Time:       "50 мин",
		Difficulty: "средне",
		PhotoURL:   "https://www.themealdb.com/images/media/meals/wyxwsp1486979827.jpg",
		SourceURL:  "https://www.themealdb.com/meal/52795-chicken-handi-recipe",
		Ingredients: []string{
			"Курица - 1,2 кг",
			"Лук - 5 тонко нарезанных",
			"Томаты - 2 мелко нарезанных",
			"Чеснок - 8 зубчиков",
			"Имбирная паста - 1 ст. л.",
			"Растительное масло - 1/4 стакана",
			"Зира - 2 ч. л.",
			"Семена кориандра - 3 ч. л.",
			"Куркума - 1 ч. л.",
			"Порошок чили - 1 ч. л.",
			"Йогурт - 1 стакан",
			"Сливки - 3/4 стакана",
			"Гарам масала - 1 ч. л.",
		},
		Steps: []string{
			"Обжарьте лук до золотистого цвета и отложите.",
			"Прогрейте чеснок и томаты, затем добавьте имбирь и специи.",
			"Добавьте курицу и готовьте, пока мясо не схватится и не станет мягким.",
			"Убавьте огонь, вмешайте йогурт, затем добавьте сливки, пажитник и гарам масалу.",
			"Подавайте горячей с naan или roti.",
		},
	},
	{
		ID:         "beef-pie",
		Name:       "Пирог с говядиной и горчицей",
		Area:       "Британия",
		Category:   "Говядина",
		Time:       "2 ч 30 мин",
		Difficulty: "средне",
		PhotoURL:   "https://www.themealdb.com/images/media/meals/sytuqu1511553755.jpg",
		SourceURL:  "https://www.themealdb.com/meal/52874-beef-and-mustard-pie-recipe",
		Ingredients: []string{
			"Говядина - 1 кг",
			"Пшеничная мука - 2 ст. л.",
			"Рапсовое масло - 2 ст. л.",
			"Красное вино - 200 мл",
			"Говяжий бульон - 400 мл",
			"Лук - 1 нарезанный",
			"Морковь - 2 нарезанные",
			"Тимьян - 3 веточки",
			"Горчица - 2 ст. л.",
			"Слоеное тесто - 400 г",
			"Желтки - 2",
			"Зеленая фасоль - 300 г",
		},
		Steps: []string{
			"Обжарьте говядину в муке на масле, затем соберите соус с вином и бульоном.",
			"Томите с луком, морковью, тимьяном и горчицей до мягкости.",
			"Остудите начинку, накройте слоеным тестом и смажьте желтком.",
			"Запекайте до золотистой корочки и подавайте с фасолью в сливочном масле.",
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
	case "/start", ReplyButtons, ReplyRecipes, "Buttons", "Recipes":
		return b.sendStart(message.Chat.ID)
	case ReplyRandomRecipe, "Random recipe":
		return b.sendRecipePhoto(message.Chat.ID, b.randomRecipe())
	case ReplyDevTools, "Dev tools":
		return b.sendDevTools(message.Chat.ID)
	case ReplyPing, "Ping":
		_, err := b.client.SendMessage(&telego.SendMessageParams{
			ChatID: telego.ChatID{ID: message.Chat.ID},
			Text:   "Понг. Эмулятор получил сообщение, а бот-рецептов ответил на него.",
		})
		return err
	case ReplyTraceError, "Trace error":
		return b.triggerErrorScenario(message.Chat.ID)
	default:
		if text == "" {
			text = "<пусто>"
		}
		_, err := b.client.SendMessage(&telego.SendMessageParams{
			ChatID: telego.ChatID{ID: message.Chat.ID},
			Text:   "Эхо: " + text,
			ReplyMarkup: &telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{
				{{Text: "Открыть рецепты", CallbackData: CallbackRecipeList}},
				{{Text: "Инструменты", CallbackData: CallbackDevTools}},
			}},
		})
		return err
	}
}

func (b *Bot) handleFoodPhoto(message *telego.Message) error {
	text := "Фото получено как Telegram photo update."
	if strings.TrimSpace(message.Caption) != "" {
		text += "\nПодпись: " + strings.TrimSpace(message.Caption)
	}
	text += "\nВыберите идею рецепта из демо-каталога:"
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
		if err := b.answer(query.ID, "Открываю рецепты"); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Не удалось определить чат для списка рецептов")
		}
		return b.sendRecipeList(chatID)
	case query.Data == CallbackRecipeRandom:
		if err := b.answer(query.ID, "Выбираю рецепт"); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Не удалось определить чат для рецепта")
		}
		return b.sendRecipePhoto(chatID, b.randomRecipe())
	case strings.HasPrefix(query.Data, CallbackRecipePrefix):
		if err := b.answer(query.ID, "Отправляю карточку рецепта"); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Не удалось определить чат для рецепта")
		}
		found, ok := findRecipe(strings.TrimPrefix(query.Data, CallbackRecipePrefix))
		if !ok {
			return b.alert(query.ID, "Рецепт не найден")
		}
		return b.sendRecipePhoto(chatID, found)
	case strings.HasPrefix(query.Data, CallbackIngredients):
		if err := b.answer(query.ID, "Открываю ингредиенты"); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Не удалось определить чат для ингредиентов")
		}
		found, ok := findRecipe(strings.TrimPrefix(query.Data, CallbackIngredients))
		if !ok {
			return b.alert(query.ID, "Рецепт не найден")
		}
		return b.sendRecipeDetails(chatID, "Ингредиенты: "+found.Name, found.Ingredients)
	case strings.HasPrefix(query.Data, CallbackSteps):
		if err := b.answer(query.ID, "Открываю шаги"); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Не удалось определить чат для шагов")
		}
		found, ok := findRecipe(strings.TrimPrefix(query.Data, CallbackSteps))
		if !ok {
			return b.alert(query.ID, "Рецепт не найден")
		}
		return b.sendRecipeDetails(chatID, "Шаги: "+found.Name, found.Steps)
	case query.Data == CallbackDevTools:
		if err := b.answer(query.ID, "Открываю инструменты"); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Не удалось определить чат для инструментов")
		}
		return b.sendDevTools(chatID)
	case query.Data == CallbackEdit:
		if err := b.client.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Редактирую исходное сообщение бота",
		}); err != nil {
			return err
		}
		msg, ok := accessibleCallbackMessage(query)
		if !ok {
			return b.alert(query.ID, "Сообщение callback недоступно")
		}
		_, err := b.client.EditMessageText(&telego.EditMessageTextParams{
			ChatID:    telego.ChatID{ID: msg.Chat.ID},
			MessageID: msg.MessageID,
			Text:      "Витринный бот отредактировал сообщение в " + b.now().Format("15:04:05"),
			ReplyMarkup: &telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{
				{{Text: "Назад к инструментам", CallbackData: CallbackDevTools}},
			}},
		})
		return err
	case query.Data == CallbackToast:
		return b.client.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Toast из answerCallbackQuery",
		})
	case query.Data == CallbackDeleteTemp:
		if err := b.client.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Создаю и удаляю временное сообщение",
		}); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Не удалось определить чат для временного сообщения")
		}
		sent, err := b.client.SendMessage(&telego.SendMessageParams{
			ChatID: telego.ChatID{ID: chatID},
			Text:   "Временное сообщение. Оно должно сразу исчезнуть.",
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
			Text:            "Reply keyboard отправлена",
		}); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Не удалось определить чат для reply keyboard")
		}
		return b.sendReplyKeyboard(chatID)
	case query.Data == CallbackError:
		if err := b.client.AnswerCallbackQuery(&telego.AnswerCallbackQueryParams{
			CallbackQueryID: query.ID,
			Text:            "Создаю видимую trace-ошибку",
			ShowAlert:       true,
		}); err != nil {
			return err
		}
		chatID, ok := callbackChatID(query)
		if !ok {
			return b.alert(query.ID, "Не удалось определить чат для trace-ошибки")
		}
		return b.triggerErrorScenario(chatID)
	default:
		return b.alert(query.ID, "Неизвестный callback витрины: "+query.Data)
	}
}

func (b *Bot) sendStart(chatID int64) error {
	_, err := b.client.SendMessage(&telego.SendMessageParams{
		ChatID: telego.ChatID{ID: chatID},
		Text: strings.Join([]string{
			"Бот-рецептов готов.",
			"Открывайте карточки рецептов, смотрите ингредиенты и шаги, прикрепляйте фото еды или используйте инструменты для проверки callbacks, edit/delete, reply keyboard и trace-ошибок.",
		}, "\n"),
		ReplyMarkup: startInlineKeyboard(),
	})
	return err
}

func (b *Bot) sendRecipeList(chatID int64) error {
	_, err := b.client.SendMessage(&telego.SendMessageParams{
		ChatID:      telego.ChatID{ID: chatID},
		Text:        "Выберите рецепт из демо-каталога:",
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
			"Время: " + item.Time + " | Сложность: " + item.Difficulty,
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
			{{Text: "Назад к рецептам", CallbackData: CallbackRecipeList}},
			{{Text: "Инструменты", CallbackData: CallbackDevTools}},
		}},
	})
	return err
}

func (b *Bot) sendReplyKeyboard(chatID int64) error {
	_, err := b.client.SendMessage(&telego.SendMessageParams{
		ChatID: telego.ChatID{ID: chatID},
		Text:   "Reply keyboard активна. Нажмите команду ниже или напишите любой текст для эхо-ответа.",
		ReplyMarkup: &telego.ReplyKeyboardMarkup{
			Keyboard: [][]telego.KeyboardButton{
				{{Text: ReplyRecipes}, {Text: ReplyRandomRecipe}},
				{{Text: ReplyPing}, {Text: ReplyDevTools}},
				{{Text: ReplyTraceError}},
			},
			ResizeKeyboard:        true,
			InputFieldPlaceholder: "Попробуйте Рецепты, Случайный рецепт или Ошибка trace",
		},
	})
	return err
}

func (b *Bot) sendDevTools(chatID int64) error {
	_, err := b.client.SendMessage(&telego.SendMessageParams{
		ChatID: telego.ChatID{ID: chatID},
		Text: strings.Join([]string{
			"Инструменты разработчика готовы.",
			"Эти кнопки проверяют Bot API callbacks, редактирование, удаление, reply keyboard и trace-ошибки.",
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
		Text:   "Отправлен намеренно неправильный Bot API вызов. В консоли он должен появиться как ошибка.",
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
			{Text: "Смотреть рецепты", CallbackData: CallbackRecipeList},
			{Text: "Удиви меня", CallbackData: CallbackRecipeRandom},
		},
		{
			{Text: "Reply keyboard", CallbackData: CallbackReply},
			{Text: "Инструменты", CallbackData: CallbackDevTools},
		},
		{
			{Text: "Ошибка trace", CallbackData: CallbackError},
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
		[]telego.InlineKeyboardButton{{Text: "Удиви меня", CallbackData: CallbackRecipeRandom}},
		[]telego.InlineKeyboardButton{{Text: "Инструменты", CallbackData: CallbackDevTools}},
	)
	return &telego.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func recipeDetailKeyboard(item recipe) *telego.InlineKeyboardMarkup {
	return &telego.InlineKeyboardMarkup{InlineKeyboard: [][]telego.InlineKeyboardButton{
		{
			{Text: "Ингредиенты", CallbackData: CallbackIngredients + item.ID},
			{Text: "Шаги", CallbackData: CallbackSteps + item.ID},
		},
		{
			{Text: "Источник", URL: item.SourceURL},
			{Text: "Другой", CallbackData: CallbackRecipeRandom},
		},
		{
			{Text: "Все рецепты", CallbackData: CallbackRecipeList},
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
			{Text: "Удалить временное", CallbackData: CallbackDeleteTemp},
			{Text: "Reply keyboard", CallbackData: CallbackReply},
		},
		{
			{Text: "Ошибка trace", CallbackData: CallbackError},
			{Text: "Рецепты", CallbackData: CallbackRecipeList},
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
