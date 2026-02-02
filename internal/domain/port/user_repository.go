package port

import (
	"context"

	"vision-bot/internal/domain/entity"
)

// UserRepository интерфейс хранилища пользователей
type UserRepository interface {
	// Get возвращает пользователя по ID, создаёт нового если не найден
	Get(ctx context.Context, userID, chatID int64) (*entity.User, error)

	// Save сохраняет состояние пользователя
	Save(ctx context.Context, user *entity.User) error

	// UpdateState обновляет состояние пользователя
	UpdateState(ctx context.Context, userID int64, state entity.UserState) error
}
