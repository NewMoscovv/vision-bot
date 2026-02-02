package entity

// UserState состояние пользователя в диалоге
type UserState string

const (
	StateMainMenu      UserState = "main_menu"      // В главном меню
	StateAwaitingPhoto UserState = "awaiting_photo" // Ожидание фото детали
	StateProcessing    UserState = "processing"     // Обработка изображения
)

// User представляет пользователя бота
type User struct {
	ID     int64     // Telegram User ID
	ChatID int64     // Telegram Chat ID
	State  UserState // Текущее состояние пользователя
}

// NewUser создаёт нового пользователя с начальным состоянием
func NewUser(userID, chatID int64) *User {
	return &User{
		ID:     userID,
		ChatID: chatID,
		State:  StateMainMenu,
	}
}

// SetState обновляет состояние пользователя
func (u *User) SetState(state UserState) {
	u.State = state
}
