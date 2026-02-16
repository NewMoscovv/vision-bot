package telegram

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"vision-bot/internal/container"
	"vision-bot/internal/domain/entity"
)

// Bot представляет Telegram-бота и хранит доступ к сервисам приложения.
type Bot struct {
	api       *tgbotapi.BotAPI
	container *container.Container
}

// NewBot создаёт нового бота и подключает контейнер сервисов.
func NewBot(token string, container *container.Container) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}

	log.Printf("Authorized on account %s", api.Self.UserName)

	return &Bot{
		api:       api,
		container: container,
	}, nil
}

// Run запускает основной цикл обработки сообщений от Telegram.
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

// handleMessage выбирает сценарий в зависимости от состояния пользователя.
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

// sendMessage отправляет текстовое сообщение в чат.
func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("Error sending message: %v", err)
	}
}

// sendPhoto отправляет изображение в чат.
func (b *Bot) sendPhoto(chatID int64, imageData []byte) {
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{
		Name:  "result.jpg",
		Bytes: imageData,
	})
	if _, err := b.api.Send(photo); err != nil {
		log.Printf("Error sending photo: %v", err)
	}
}

// handleMainMenu обрабатывает сообщения в состоянии главного меню.
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

// handleAwaitingOriginal обрабатывает сообщения при ожидании оригинального фото.
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

// handleAwaitingDefect обрабатывает сообщения при ожидании фото дефекта.
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
	go b.processDefectPhoto(msg.From.ID, msg.Chat.ID, photoData)
}

// processDefectPhoto запускает детектор дефектов и отправляет результат.
func (b *Bot) processDefectPhoto(userID int64, chatID int64, photo []byte) {
	result, err := b.container.InspectionService.ProcessDefectPhotoDiff(context.Background(), userID, photo)
	if err != nil {
		log.Printf(
			"ProcessDefectPhoto failed user_id=%d chat_id=%d reason=%s err=%v",
			userID,
			chatID,
			classifyInspectionError(err),
			err,
		)
		b.sendMessage(chatID, msgProcessingError)
		return
	}
	if result == nil || result.Result == nil {
		log.Printf(
			"ProcessDefectPhoto failed user_id=%d chat_id=%d reason=empty_result",
			userID,
			chatID,
		)
		b.sendMessage(chatID, msgProcessingError)
		return
	}

	log.Printf(
		"ProcessDefectPhoto completed user_id=%d chat_id=%d has_defects=%t defects=%d",
		userID,
		chatID,
		result.Result.HasDefects,
		len(result.Result.Defects),
	)
	for i, defect := range result.Result.Defects {
		reason := defect.Reason
		if reason == "" {
			reason = "not_set"
		}
		log.Printf(
			"ProcessDefectPhoto defect user_id=%d chat_id=%d idx=%d bbox=(x=%d y=%d w=%d h=%d area=%d) reason=%s",
			userID,
			chatID,
			i,
			defect.X,
			defect.Y,
			defect.Width,
			defect.Height,
			defect.Area,
			reason,
		)
	}

	if result.Result.HasDefects {
		b.sendMessage(chatID, msgDefectsFound)
		if len(result.Highlighted) > 0 {
			b.sendPhoto(chatID, result.Highlighted)
		}
		return
	}

	b.sendMessage(chatID, msgNoDefects)
}

// extractPhoto извлекает фото из сообщения и скачивает его.
func (b *Bot) extractPhoto(msg *tgbotapi.Message) ([]byte, error) {
	if msg.Photo == nil || len(msg.Photo) == 0 {
		return nil, nil
	}

	photo := msg.Photo[len(msg.Photo)-1]
	return b.downloadFile(photo.FileID)
}

// downloadFile скачивает файл из Telegram по его ID.
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

func classifyInspectionError(err error) string {
	if err == nil {
		return "none"
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "quality gate failed"):
		return "quality_gate"
	case strings.Contains(msg, "alignment failed"):
		return "alignment"
	case strings.Contains(msg, "failed to decode"):
		return "decode"
	case strings.Contains(msg, "original photo is not found"):
		return "missing_original"
	case strings.Contains(msg, "detector is not configured"):
		return "detector_not_configured"
	case strings.Contains(msg, "empty image"):
		return "empty_image"
	default:
		return "unknown"
	}
}
