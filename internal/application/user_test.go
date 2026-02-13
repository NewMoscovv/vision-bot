package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"vision-bot/internal/domain/entity"
	"vision-bot/internal/infrastructure/storage"
)

func TestUserService_BeginCheckAndCancel(t *testing.T) {
	repo := storage.NewMemoryUserRepository()
	svc := NewUserService(repo)
	ctx := context.Background()

	user, err := svc.BeginCheck(ctx, 1, 10)
	require.NoError(t, err)
	require.Equal(t, entity.StateAwaitingOriginalPhoto, user.State)

	user, err = svc.Cancel(ctx, 1, 10)
	require.NoError(t, err)
	require.Equal(t, entity.StateMainMenu, user.State)
}

func TestUserService_SetState(t *testing.T) {
	repo := storage.NewMemoryUserRepository()
	svc := NewUserService(repo)
	ctx := context.Background()

	user, err := svc.SetState(ctx, 2, 20, entity.StateAwaitingDefectPhoto)
	require.NoError(t, err)
	require.Equal(t, entity.StateAwaitingDefectPhoto, user.State)
}
