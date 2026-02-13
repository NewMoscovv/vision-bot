package app

import (
	"context"
	"errors"
	"sync"

	"vision-bot/internal/domain/entity"
	"vision-bot/internal/domain/port"
)

type InspectionService struct {
	users     *UserService
	detector  port.DefectDetector
	describer port.DefectDescriber
	originals map[int64][]byte
	mu        sync.RWMutex
}

// InspectionOutput содержит результат поиска дефектов и картинку с подсветкой.
type InspectionOutput struct {
	Result      *entity.InspectionResult
	Highlighted []byte
}

// NewInspectionService создаёт сервис, который управляет проверкой дефектов.
func NewInspectionService(users *UserService, detector port.DefectDetector, describer port.DefectDescriber) *InspectionService {
	return &InspectionService{
		users:     users,
		detector:  detector,
		describer: describer,
		originals: make(map[int64][]byte),
	}
}

// AcceptOriginalPhoto принимает оригинальное фото и переводит пользователя дальше по сценарию.
func (s *InspectionService) AcceptOriginalPhoto(ctx context.Context, userID, chatID int64, photo []byte) (*entity.User, error) {
	// Сохраняем оригинал в памяти, чтобы сравнить со следующей фотографией.
	s.mu.Lock()
	s.originals[userID] = photo
	s.mu.Unlock()
	return s.users.SetState(ctx, userID, chatID, entity.StateAwaitingDefectPhoto)
}

// AcceptDefectPhoto принимает фото дефекта и возвращает пользователя в главное меню.
func (s *InspectionService) AcceptDefectPhoto(ctx context.Context, userID, chatID int64, photo []byte) (*entity.User, error) {
	_ = photo
	return s.users.SetState(ctx, userID, chatID, entity.StateMainMenu)
}

// ProcessDefectPhotoDiff сравнивает эталон и текущее фото и возвращает результат.
func (s *InspectionService) ProcessDefectPhotoDiff(ctx context.Context, userID int64, current []byte) (*InspectionOutput, error) {
	if s.detector == nil {
		return nil, errors.New("detector is not configured")
	}

	s.mu.RLock()
	base, ok := s.originals[userID]
	s.mu.RUnlock()
	if !ok || len(base) == 0 {
		return nil, errors.New("original photo is not found")
	}

	result, err := s.detector.InspectDiff(ctx, base, current)
	if err != nil {
		return nil, err
	}

	var highlighted []byte
	if result.HasDefects {
		highlighted, _ = s.detector.HighlightDefects(current, result)
	}

	_ = s.describer
	return &InspectionOutput{Result: result, Highlighted: highlighted}, nil
}
// ProcessDefectPhoto запускает детектор и возвращает результат с подсветкой.
func (s *InspectionService) ProcessDefectPhoto(ctx context.Context, photo []byte) (*InspectionOutput, error) {
	if s.detector == nil {
		return nil, errors.New("detector is not configured")
	}

	result, err := s.detector.Inspect(ctx, photo)
	if err != nil {
		return nil, err
	}

	var highlighted []byte
	if result.HasDefects {
		highlighted, _ = s.detector.HighlightDefects(photo, result)
	}

	_ = s.describer
	return &InspectionOutput{Result: result, Highlighted: highlighted}, nil
}
