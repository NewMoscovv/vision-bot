//go:build !gocv
// +build !gocv

package vision

import (
	"context"
	"errors"

	"vision-bot/internal/domain/entity"
)

type GoCVDetector struct {
	MinAreaRatio   float64
	MaxAspectRatio float64
	MinAspectRatio float64
	MaxSide        int
}

// NewGoCVDetector создаёт детектор-заглушку (без OpenCV).
func NewGoCVDetector(minArea int) *GoCVDetector {
	return &GoCVDetector{
		MinAreaRatio:   0.001,
		MinAspectRatio: 0.1,
		MaxAspectRatio: 10.0,
		MaxSide:        1024,
	}
}

// Inspect возвращает ошибку, если сборка без тега gocv.
func (d *GoCVDetector) Inspect(ctx context.Context, imageData []byte) (*entity.InspectionResult, error) {
	_ = ctx
	_ = imageData
	return nil, errors.New("gocv build tag is not enabled")
}

// InspectDiff возвращает ошибку, если сборка без тега gocv.
func (d *GoCVDetector) InspectDiff(ctx context.Context, baseImage []byte, currentImage []byte) (*entity.InspectionResult, error) {
	_ = ctx
	_ = baseImage
	_ = currentImage
	return nil, errors.New("gocv build tag is not enabled")
}

// HighlightDefects возвращает ошибку, если сборка без тега gocv.
func (d *GoCVDetector) HighlightDefects(imageData []byte, result *entity.InspectionResult) ([]byte, error) {
	_ = imageData
	_ = result
	return nil, errors.New("gocv build tag is not enabled")
}
