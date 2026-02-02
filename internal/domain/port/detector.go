package port

import (
	"context"

	"vision-bot/internal/domain/entity"
)

// DefectDetector интерфейс детектора дефектов
type DefectDetector interface {
	// Inspect анализирует изображение и возвращает результат инспекции
	Inspect(ctx context.Context, imageData []byte) (*entity.InspectionResult, error)

	// HighlightDefects создаёт изображение с подсветкой дефектов
	HighlightDefects(imageData []byte, result *entity.InspectionResult) ([]byte, error)
}
