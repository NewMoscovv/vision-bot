package storage

import (
	"context"
	"sync"

	"vision-bot/internal/domain/entity"
	"vision-bot/internal/domain/port"
)

// MemoryUserRepository in-memory хранилище пользователей
type MemoryUserRepository struct {
	mu    sync.RWMutex
	users map[int64]*entity.User
}

// NewMemoryUserRepository создаёт новое in-memory хранилище
func NewMemoryUserRepository() *MemoryUserRepository {
	return &MemoryUserRepository{
		users: make(map[int64]*entity.User),
	}
}

// Get возвращает пользователя по ID, создаёт нового если не найден
func (r *MemoryUserRepository) Get(ctx context.Context, userID, chatID int64) (*entity.User, error) {
	r.mu.RLock()
	user, exists := r.users[userID]
	r.mu.RUnlock()

	if exists {
		return user, nil
	}

	// Создаём нового пользователя
	newUser := entity.NewUser(userID, chatID)

	r.mu.Lock()
	r.users[userID] = newUser
	r.mu.Unlock()

	return newUser, nil
}

// Save сохраняет состояние пользователя
func (r *MemoryUserRepository) Save(ctx context.Context, user *entity.User) error {
	r.mu.Lock()
	r.users[user.ID] = user
	r.mu.Unlock()

	return nil
}

// UpdateState обновляет состояние пользователя
func (r *MemoryUserRepository) UpdateState(ctx context.Context, userID int64, state entity.UserState) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if user, exists := r.users[userID]; exists {
		user.SetState(state)
	}

	return nil
}

// Проверка реализации интерфейса
var _ port.UserRepository = (*MemoryUserRepository)(nil)
