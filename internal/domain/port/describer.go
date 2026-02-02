package port

import (
	"context"

	"vision-bot/internal/domain/entity"
)

// DefectDescriber интерфейс описателя дефектов
type DefectDescriber interface {
	// Describe генерирует текстовое описание найденных дефектов
	Describe(ctx context.Context, result *entity.InspectionResult) (*entity.AiDescription, error)
}
