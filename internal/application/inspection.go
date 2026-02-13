package app

import (
	"context"

	"vision-bot/internal/domain/entity"
	"vision-bot/internal/domain/port"
)

type InspectionService struct {
	users     *UserService
	detector  port.DefectDetector
	describer port.DefectDescriber
}

func NewInspectionService(users *UserService, detector port.DefectDetector, describer port.DefectDescriber) *InspectionService {
	return &InspectionService{
		users:     users,
		detector:  detector,
		describer: describer,
	}
}

func (s *InspectionService) AcceptOriginalPhoto(ctx context.Context, userID, chatID int64, photo []byte) (*entity.User, error) {
	_ = photo // TODO: сохранить оригинал, если нужно
	return s.users.SetState(ctx, userID, chatID, entity.StateAwaitingDefectPhoto)
}

func (s *InspectionService) AcceptDefectPhoto(ctx context.Context, userID, chatID int64, photo []byte) (*entity.User, error) {
	_ = photo // TODO: вызвать детектор и описатель
	_ = s.detector
	_ = s.describer
	return s.users.SetState(ctx, userID, chatID, entity.StateMainMenu)
}
