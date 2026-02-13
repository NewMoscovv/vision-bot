package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"vision-bot/internal/domain/entity"
	"vision-bot/internal/infrastructure/storage"
)

func TestInspectionService_AcceptOriginalPhoto(t *testing.T) {
	repo := storage.NewMemoryUserRepository()
	userSvc := NewUserService(repo)
	svc := NewInspectionService(userSvc, nil, nil)
	ctx := context.Background()

	user, err := svc.AcceptOriginalPhoto(ctx, 1, 10, []byte("orig"))
	require.NoError(t, err)
	require.Equal(t, entity.StateAwaitingDefectPhoto, user.State)
}

func TestInspectionService_AcceptDefectPhoto(t *testing.T) {
	repo := storage.NewMemoryUserRepository()
	userSvc := NewUserService(repo)
	svc := NewInspectionService(userSvc, nil, nil)
	ctx := context.Background()

	user, err := svc.AcceptDefectPhoto(ctx, 1, 10, []byte("defect"))
	require.NoError(t, err)
	require.Equal(t, entity.StateMainMenu, user.State)
}
