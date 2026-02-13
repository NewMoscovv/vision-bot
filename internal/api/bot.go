package telegram

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"vision-bot/internal/container"
	"vision-bot/internal/domain/entity"
)


// Bot представляет Telegram-бота
type Bot struct {
	api               *tgbotapi.BotAPI
	container *container.Container
}

// NewBot создаёт нового бота
func NewBot(token string, container *container.Container) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}

	log.Printf("Authorized on account %s", api.Self.UserName)

	return &Bot{
		api:               api,
		container: container,
	}, nil
}

// Run запускает основной цикл обработки сообщений
func (b *Bot) Run() error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	ctx := context.Background()

	for update := range updates {
		if update.Message == nil {
			continue
		}

		b.handleMessage(ctx, update.Message)
	}

	return nil
}

// handleMessage обрабатывает входящее сообщение
func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	user, err := b.container.UserService.Get(ctx, msg.From.ID, msg.Chat.ID)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		b.sendMessage(msg.Chat.ID, msgProcessingError)
		return
	}

	switch user.State {
	case entity.StateMainMenu:
		b.handleMainMenu(ctx, msg)
		return
	case entity.StateAwaitingOriginalPhoto:
		b.handleAwaitingOriginal(ctx, msg)
		return
	case entity.StateAwaitingDefectPhoto:
		b.handleAwaitingDefect(ctx, msg)
		return
	default:
		_, _ = b.container.UserService.Cancel(ctx, msg.From.ID, msg.Chat.ID)
		b.sendMessage(msg.Chat.ID, msgStart)
		return
	}
}

// sendMessage отправляет текстовое сообщение
func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("Error sending message: %v", err)
	}
}

// sendPhoto отправляет изображение
func (b *Bot) sendPhoto(chatID int64, imageData []byte) {
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{
		Name:  "result.jpg",
		Bytes: imageData,
	})
	if _, err := b.api.Send(photo); err != nil {
		log.Printf("Error sending photo: %v", err)
	}
}

// handleMainMenu обрабатывает сообщения в состоянии главного меню
func (b *Bot) handleMainMenu(ctx context.Context, msg *tgbotapi.Message) {
	if msg.IsCommand() {
		switch msg.Command() {
		case cmdStart:
			b.sendMessage(msg.Chat.ID, msgStart)
			return
		case cmdHelp:
			b.sendMessage(msg.Chat.ID, msgHelp)
			return
		case cmdCheck:
			if _, err := b.container.UserService.BeginCheck(ctx, msg.From.ID, msg.Chat.ID); err != nil {
				log.Printf("BeginCheck error: %v", err)
				b.sendMessage(msg.Chat.ID, msgProcessingError)
				return
			}
			b.sendMessage(msg.Chat.ID, msgAwaitingOriginal)
			return
		default:
			b.sendMessage(msg.Chat.ID, msgStart)
			return
		}
	}

	b.sendMessage(msg.Chat.ID, msgStart)
}

// handleAwaitingOriginal обрабатывает сообщения в состоянии ожидания оригинала
func (b *Bot) handleAwaitingOriginal(ctx context.Context, msg *tgbotapi.Message) {
	if msg.IsCommand() {
		if msg.Command() == cmdCancel {
			if _, err := b.container.UserService.Cancel(ctx, msg.From.ID, msg.Chat.ID); err != nil {
				log.Printf("Cancel error: %v", err)
				b.sendMessage(msg.Chat.ID, msgProcessingError)
				return
			}
			b.sendMessage(msg.Chat.ID, msgCancelled)
			return
		}
		b.sendMessage(msg.Chat.ID, msgOnlyCancel)
		return
	}

	photoData, err := b.extractPhoto(msg)
	if err != nil {
		log.Printf("Error downloading original photo: %v", err)
		b.sendMessage(msg.Chat.ID, msgProcessingError)
		return
	}
	if len(photoData) == 0 {
		b.sendMessage(msg.Chat.ID, msgAwaitingOriginal)
		return
	}

	if _, err := b.container.InspectionService.AcceptOriginalPhoto(ctx, msg.From.ID, msg.Chat.ID, photoData); err != nil {
		log.Printf("AcceptOriginalPhoto error: %v", err)
		b.sendMessage(msg.Chat.ID, msgProcessingError)
		return
	}

	b.sendMessage(msg.Chat.ID, msgAwaitingDefect)
}

// handleAwaitingDefect обрабатывает сообщения в состоянии ожидания дефекта
func (b *Bot) handleAwaitingDefect(ctx context.Context, msg *tgbotapi.Message) {
	if msg.IsCommand() {
		if msg.Command() == cmdCancel {
			if _, err := b.container.UserService.Cancel(ctx, msg.From.ID, msg.Chat.ID); err != nil {
				log.Printf("Cancel error: %v", err)
				b.sendMessage(msg.Chat.ID, msgProcessingError)
				return
			}
			b.sendMessage(msg.Chat.ID, msgCancelled)
			return
		}
		b.sendMessage(msg.Chat.ID, msgOnlyCancel)
		return
	}

	photoData, err := b.extractPhoto(msg)
	if err != nil {
		log.Printf("Error downloading defect photo: %v", err)
		b.sendMessage(msg.Chat.ID, msgProcessingError)
		return
	}
	if len(photoData) == 0 {
		b.sendMessage(msg.Chat.ID, msgAwaitingDefect)
		return
	}

	if _, err := b.container.InspectionService.AcceptDefectPhoto(ctx, msg.From.ID, msg.Chat.ID, photoData); err != nil {
		log.Printf("AcceptDefectPhoto error: %v", err)
		b.sendMessage(msg.Chat.ID, msgProcessingError)
		return
	}

	b.sendMessage(msg.Chat.ID, msgProcessing)
	b.sendMessage(msg.Chat.ID, "Получено изображение дефекта. Обработка пока не реализована.")
}

func (b *Bot) extractPhoto(msg *tgbotapi.Message) ([]byte, error) {
	if msg.Photo == nil || len(msg.Photo) == 0 {
		return nil, nil
	}

	photo := msg.Photo[len(msg.Photo)-1]
	return b.downloadFile(photo.FileID)
}

// downloadFile скачивает файл из Telegram
func (b *Bot) downloadFile(fileID string) ([]byte, error) {
	file, err := b.api.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return nil, fmt.Errorf("get file: %w", err)
	}

	fileURL := file.Link(b.api.Token)

	resp, err := http.Get(fileURL)
	if err != nil {
		return nil, fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return data, nil
}
