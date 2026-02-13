package app

import (
	"context"

	"vision-bot/internal/domain/entity"
	"vision-bot/internal/domain/port"
)

type UserService struct {
	repo port.UserRepository
}

func NewUserService(repo port.UserRepository) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) Get(ctx context.Context, userID, chatID int64) (*entity.User, error) {
	return s.repo.Get(ctx, userID, chatID)
}

func (s *UserService) SetState(ctx context.Context, userID, chatID int64, state entity.UserState) (*entity.User, error) {
	user, err := s.repo.Get(ctx, userID, chatID)
	if err != nil {
		return nil, err
	}

	user.SetState(state)
	if err := s.repo.Save(ctx, user); err != nil {
		return nil, err
	}

	return user, nil
}

func (s *UserService) BeginCheck(ctx context.Context, userID, chatID int64) (*entity.User, error) {
	return s.SetState(ctx, userID, chatID, entity.StateAwaitingOriginalPhoto)
}

func (s *UserService) Cancel(ctx context.Context, userID, chatID int64) (*entity.User, error) {
	return s.SetState(ctx, userID, chatID, entity.StateMainMenu)
}
