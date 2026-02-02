package telegram

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"vision-bot/internal/domain/entity"
	"vision-bot/internal/domain/port"
)

const (
	msgStart = `👋 Привет! Я бот для поиска дефектов на фотографиях деталей.

📸 Отправьте мне фото детали, и я попробую найти и описать дефекты.

📋 Команды:
/check — начать проверку детали
/help — справка
/cancel — отменить текущую операцию`

	msgHelp = `ℹ️ Как пользоваться ботом:

1️⃣ Отправьте фото детали
2️⃣ Бот проанализирует изображение
3️⃣ Вы получите результат: текст + фото с подсветкой дефектов

💡 Рекомендации:
• Снимайте при хорошем освещении
• Используйте однотонный фон
• Фото должно быть чётким

📋 Команды:
/check — начать проверку
/cancel — отменить операцию`

	msgAwaitingPhoto   = "📸 Отправьте фото детали для проверки на дефекты."
	msgCancelled       = "❌ Операция отменена. Отправьте /check для новой проверки."
	msgSendPhoto       = "📸 Пожалуйста, отправьте фото детали для проверки на дефекты."
	msgUnknownCommand  = "❓ Неизвестная команда. Используйте /help для справки."
	msgProcessing      = "⏳ Обрабатываю изображение..."
	msgNoDefects       = "✅ Дефекты не обнаружены."
	msgProcessingError = "⚠️ Не удалось обработать изображение. Попробуйте сделать другое фото."
)

// Bot представляет Telegram-бота
type Bot struct {
	api      *tgbotapi.BotAPI
	userRepo port.UserRepository
}

// NewBot создаёт нового бота
func NewBot(token string, userRepo port.UserRepository) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}

	log.Printf("Authorized on account %s", api.Self.UserName)

	return &Bot{
		api:      api,
		userRepo: userRepo,
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
	user, err := b.userRepo.Get(ctx, msg.From.ID, msg.Chat.ID)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		return
	}

	// Обработка команд
	if msg.IsCommand() {
		b.handleCommand(ctx, msg, user)
		return
	}

	// Обработка фото
	if msg.Photo != nil && len(msg.Photo) > 0 {
		b.handlePhoto(ctx, msg, user)
		return
	}

	// Текстовое сообщение (не команда)
	b.sendMessage(msg.Chat.ID, msgSendPhoto)
}

// handleCommand обрабатывает команды бота
func (b *Bot) handleCommand(ctx context.Context, msg *tgbotapi.Message, user *entity.User) {
	switch msg.Command() {
	case "start":
		user.SetState(entity.StateMainMenu)
		b.userRepo.Save(ctx, user)
		b.sendMessage(msg.Chat.ID, msgStart)

	case "help":
		b.sendMessage(msg.Chat.ID, msgHelp)

	case "check":
		user.SetState(entity.StateAwaitingPhoto)
		b.userRepo.Save(ctx, user)
		b.sendMessage(msg.Chat.ID, msgAwaitingPhoto)

	case "cancel":
		user.SetState(entity.StateMainMenu)
		b.userRepo.Save(ctx, user)
		b.sendMessage(msg.Chat.ID, msgCancelled)

	default:
		b.sendMessage(msg.Chat.ID, msgUnknownCommand)
	}
}

// handlePhoto обрабатывает входящее фото
func (b *Bot) handlePhoto(ctx context.Context, msg *tgbotapi.Message, user *entity.User) {
	// Устанавливаем состояние "обработка"
	user.SetState(entity.StateProcessing)
	b.userRepo.Save(ctx, user)

	b.sendMessage(msg.Chat.ID, msgProcessing)

	// Получаем файл с максимальным разрешением
	photo := msg.Photo[len(msg.Photo)-1]

	imageData, err := b.downloadFile(photo.FileID)
	if err != nil {
		log.Printf("Error downloading photo: %v", err)
		b.sendMessage(msg.Chat.ID, msgProcessingError)
		user.SetState(entity.StateMainMenu)
		b.userRepo.Save(ctx, user)
		return
	}

	// TODO: Здесь будет вызов InspectionService
	// Пока просто возвращаем заглушку
	log.Printf("Received image: %d bytes", len(imageData))

	b.sendMessage(msg.Chat.ID, fmt.Sprintf("Получено изображение (%d байт). Обработка пока не реализована.", len(imageData)))

	// Возвращаем в главное меню
	user.SetState(entity.StateMainMenu)
	b.userRepo.Save(ctx, user)
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

// sendMessage отправляет текстовое сообщение
func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("Error sending message: %v", err)
	}
}
