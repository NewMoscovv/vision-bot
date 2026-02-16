//go:build !gocv
// +build !gocv

package vision

import (
	"context"
	"errors"

	"vision-bot/internal/domain/entity"
)

type GoCVDetector struct {
	MinAreaRatio                   float64
	MaxAspectRatio                 float64
	MinAspectRatio                 float64
	MaxSide                        int
	MinImageSide                   int
	MinSharpnessEdgeRatio          float64
	MaxOverexposedRatio            float64
	MaxUnderexposedRatio           float64
	MaxGlareRatio                  float64
	DiffMaxGlareRatio              float64
	MinPartAreaRatio               float64
	PartSecondaryAreaRatio         float64
	PartSecondaryRelRatio          float64
	ROIMarginKernel                int
	EnableRegistration             bool
	MinAlignmentScore              float64
	DiffMinThreshold               float32
	DiffOpenKernel                 int
	DiffCloseKernel                int
	MinContourAreaRatio            float64
	MinFillRatio                   float64
	NMSIoUThreshold                float64
	NMSContainmentRatio            float64
	BrokenMinComponentRatio        float64
	BrokenAreaLossRatio            float64
	BrokenFocusExpand              int
	BrokenMinOverlapRatio          float64
	BrokenMergeDistance            int
	BrokenSplitKernel              int
	BrokenSecondRelMin             float64
	BrokenDominantMinRatio         float64
	EnableGeometryCheck            bool
	GeometryMatchMaxScore          float64
	GeometryMinConcavity           int
	GeometryMinConcavityGap        int
	GeometryPolygonVertexGap       int
	GeometryPolygonMinCircularity  float64
	GeometryPolygonMinExtent       float64
	GeometryRoundMinCircularity    float64
	GeometryRoundMaxCircularityGap float64
	GeometryRingKernel             int
}

// NewGoCVDetector создаёт детектор-заглушку (без OpenCV).
func NewGoCVDetector(minArea int) *GoCVDetector {
	return &GoCVDetector{
		MinAreaRatio:                   0.001,
		MinAspectRatio:                 0.1,
		MaxAspectRatio:                 10.0,
		MaxSide:                        1024,
		MinImageSide:                   400,
		MinSharpnessEdgeRatio:          0.008,
		MaxOverexposedRatio:            0.35,
		MaxUnderexposedRatio:           0.45,
		MaxGlareRatio:                  0.20,
		DiffMaxGlareRatio:              0.26,
		MinPartAreaRatio:               0.05,
		PartSecondaryAreaRatio:         0.004,
		PartSecondaryRelRatio:          0.04,
		ROIMarginKernel:                9,
		EnableRegistration:             true,
		MinAlignmentScore:              0.25,
		DiffMinThreshold:               22,
		DiffOpenKernel:                 3,
		DiffCloseKernel:                7,
		MinContourAreaRatio:            0.00012,
		MinFillRatio:                   0.08,
		NMSIoUThreshold:                0.30,
		NMSContainmentRatio:            0.80,
		BrokenMinComponentRatio:        0.006,
		BrokenAreaLossRatio:            0.06,
		BrokenFocusExpand:              49,
		BrokenMinOverlapRatio:          0.10,
		BrokenMergeDistance:            48,
		BrokenSplitKernel:              17,
		BrokenSecondRelMin:             0.05,
		BrokenDominantMinRatio:         0.50,
		EnableGeometryCheck:            true,
		GeometryMatchMaxScore:          0.10,
		GeometryMinConcavity:           8,
		GeometryMinConcavityGap:        2,
		GeometryPolygonVertexGap:       2,
		GeometryPolygonMinCircularity:  0.55,
		GeometryPolygonMinExtent:       0.55,
		GeometryRoundMinCircularity:    0.82,
		GeometryRoundMaxCircularityGap: 0.10,
		GeometryRingKernel:             41,
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
