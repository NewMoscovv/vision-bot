package telegram

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api *tgbotapi.BotAPI
}

func NewBot(token string) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}

	log.Printf("Authorized on account %s", api.Self.UserName)

	return &Bot{api: api}, nil
}

func (b *Bot) Run() error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		if update.Message.IsCommand() {
			b.handleCommand(update.Message)
		}
	}

	return nil
}

func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	var text string

	switch msg.Command() {
	case "start":
		text = "Hello, vision-bot"
	case "help":
		text = "Отправьте фото детали для проверки на дефекты."
	default:
		text = "Неизвестная команда. Используйте /help"
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	if _, err := b.api.Send(reply); err != nil {
		log.Printf("Error sending message: %v", err)
	}
}
