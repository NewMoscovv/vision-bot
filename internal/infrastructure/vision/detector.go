//go:build gocv
// +build gocv

package vision

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"math"
	"sort"

	"gocv.io/x/gocv"

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

type maskComponent struct {
	rect image.Rectangle
	area float64
}

type shapeFamily string

const (
	shapeFamilyUnknown shapeFamily = "unknown"
	shapeFamilyRound   shapeFamily = "round"
	shapeFamilyPolygon shapeFamily = "polygon"
	shapeFamilyToothed shapeFamily = "toothed"
)

type shapeDescriptor struct {
	family      shapeFamily
	concavity   int
	vertices    int
	circularity float64
	extent      float64
	area        float64
	perimeter   float64
}

// NewGoCVDetector создаёт детектор с минимальной площадью дефекта.
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

// Inspect запускает анализ изображения и возвращает найденные дефекты.
func (d *GoCVDetector) Inspect(ctx context.Context, imageData []byte) (*entity.InspectionResult, error) {
	_ = ctx
	mat, err := decodeToMat(imageData)
	if err != nil {
		return nil, err
	}
	defer mat.Close()

	if mat.Empty() {
		return nil, errors.New("empty image")
	}
	if err := d.checkImageQuality(mat, "image", d.MaxGlareRatio); err != nil {
		return nil, err
	}

	// Приводим изображение к стандартному размеру для стабильных порогов.
	if mat.Cols() > d.MaxSide || mat.Rows() > d.MaxSide {
		scale := float64(d.MaxSide) / float64(maxInt(mat.Cols(), mat.Rows()))
		newW := int(float64(mat.Cols()) * scale)
		newH := int(float64(mat.Rows()) * scale)
		resized := gocv.NewMat()
		gocv.Resize(mat, &resized, image.Pt(newW, newH), 0, 0, gocv.InterpolationArea)
		mat.Close()
		mat = resized
	}

	gray := gocv.NewMat()
	defer gray.Close()
	gocv.CvtColor(mat, &gray, gocv.ColorBGRToGray)

	partMask := d.buildPartMask(mat)
	defer partMask.Close()
	roiMask := d.buildInteriorMask(partMask)
	defer roiMask.Close()

	blur := gocv.NewMat()
	defer blur.Close()
	gocv.GaussianBlur(gray, &blur, image.Pt(5, 5), 0, 0, gocv.BorderDefault)

	edges := gocv.NewMat()
	defer edges.Close()
	gocv.Canny(blur, &edges, 50, 150)
	edgeInput := edges
	maskedEdges := gocv.NewMat()
	defer maskedEdges.Close()
	if !roiMask.Empty() {
		gocv.BitwiseAnd(edges, roiMask, &maskedEdges)
		edgeInput = maskedEdges
	}

	contours := gocv.FindContours(edgeInput, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()

	defects := d.extractDefectsFromContours(contours, mat.Cols(), mat.Rows(), "edge_contour")
	d.logDefects("inspect", defects)

	return &entity.InspectionResult{
		ImageWidth:  mat.Cols(),
		ImageHeight: mat.Rows(),
		Defects:     defects,
		HasDefects:  len(defects) > 0,
	}, nil
}

// InspectDiff ищет отличия между эталоном и текущим изображением.
func (d *GoCVDetector) InspectDiff(ctx context.Context, baseImage []byte, currentImage []byte) (*entity.InspectionResult, error) {
	_ = ctx

	baseMat, err := decodeToMat(baseImage)
	if err != nil {
		return nil, err
	}
	defer baseMat.Close()

	currentMat, err := decodeToMat(currentImage)
	if err != nil {
		return nil, err
	}
	defer currentMat.Close()

	if baseMat.Empty() || currentMat.Empty() {
		return nil, errors.New("empty image")
	}
	if err := d.checkImageQuality(baseMat, "base image", d.DiffMaxGlareRatio); err != nil {
		return nil, err
	}
	if err := d.checkImageQuality(currentMat, "current image", d.DiffMaxGlareRatio); err != nil {
		return nil, err
	}

	// Приводим оба изображения к одному размеру (минимальный из двух).
	targetW := minInt(baseMat.Cols(), currentMat.Cols())
	targetH := minInt(baseMat.Rows(), currentMat.Rows())
	if baseMat.Cols() != targetW || baseMat.Rows() != targetH {
		resized := gocv.NewMat()
		gocv.Resize(baseMat, &resized, image.Pt(targetW, targetH), 0, 0, gocv.InterpolationArea)
		baseMat.Close()
		baseMat = resized
	}
	if currentMat.Cols() != targetW || currentMat.Rows() != targetH {
		resized := gocv.NewMat()
		gocv.Resize(currentMat, &resized, image.Pt(targetW, targetH), 0, 0, gocv.InterpolationArea)
		currentMat.Close()
		currentMat = resized
	}

	baseMask := d.buildPartMask(baseMat)
	defer baseMask.Close()
	currentMask := d.buildPartMask(currentMat)
	defer currentMask.Close()

	currentForDiff := currentMat
	currentMaskForROI := currentMask
	if d.EnableRegistration {
		alignedCurrent, alignedMask, alignmentScore, err := d.alignCurrentToBase(baseMat, currentMat, baseMask, currentMask)
		if err == nil {
			defer alignedCurrent.Close()
			defer alignedMask.Close()
			if alignmentScore >= d.MinAlignmentScore {
				currentForDiff = alignedCurrent
				currentMaskForROI = alignedMask
			}
		}
	}

	// Переводим в серый и считаем абсолютную разницу.
	baseGray := gocv.NewMat()
	defer baseGray.Close()
	gocv.CvtColor(baseMat, &baseGray, gocv.ColorBGRToGray)

	currentGray := gocv.NewMat()
	defer currentGray.Close()
	gocv.CvtColor(currentForDiff, &currentGray, gocv.ColorBGRToGray)

	roiMask := gocv.NewMat()
	defer roiMask.Close()
	gocv.BitwiseAnd(baseMask, currentMaskForROI, &roiMask)
	innerROIMask := d.buildInteriorMask(roiMask)
	defer innerROIMask.Close()

	diff := gocv.NewMat()
	defer diff.Close()
	gocv.AbsDiff(baseGray, currentGray, &diff)

	// Подавляем мелкий шум перед порогом.
	blur := gocv.NewMat()
	defer blur.Close()
	gocv.GaussianBlur(diff, &blur, image.Pt(5, 5), 0, 0, gocv.BorderDefault)

	// Усиливаем отличия порогом.
	thresh := gocv.NewMat()
	defer thresh.Close()
	otsuThreshold := gocv.Threshold(blur, &thresh, 0, 255, gocv.ThresholdBinary+gocv.ThresholdOtsu)
	if otsuThreshold < d.DiffMinThreshold {
		gocv.Threshold(blur, &thresh, d.DiffMinThreshold, 255, gocv.ThresholdBinary)
	}

	cleanedThresh := d.postProcessDiffMask(thresh)
	defer cleanedThresh.Close()

	brokenMode, structuralMask := d.detectBrokenPartMask(baseMask, currentMaskForROI)
	defer structuralMask.Close()
	structuralInput := structuralMask
	maskedStructural := gocv.NewMat()
	defer maskedStructural.Close()
	if brokenMode && !innerROIMask.Empty() && !structuralMask.Empty() {
		gocv.BitwiseAnd(structuralMask, innerROIMask, &maskedStructural)
		structuralInput = maskedStructural
	}
	geometryMode, geometryMask, geometryReason := d.detectGeometryMismatch(baseMask, currentMaskForROI)
	defer geometryMask.Close()
	if geometryMode && !brokenMode {
		geometryInput := geometryMask
		maskedGeometry := gocv.NewMat()
		defer maskedGeometry.Close()
		if !innerROIMask.Empty() && !geometryMask.Empty() {
			gocv.BitwiseAnd(geometryMask, innerROIMask, &maskedGeometry)
			if !maskedGeometry.Empty() && gocv.CountNonZero(maskedGeometry) > 0 {
				geometryInput = maskedGeometry
			}
		}

		defects := d.buildGeometryMismatchDefects(geometryInput, targetW, targetH, geometryReason)
		d.logDefects("inspect_diff_geometry", defects)
		return &entity.InspectionResult{
			ImageWidth:  targetW,
			ImageHeight: targetH,
			Defects:     defects,
			HasDefects:  len(defects) > 0,
		}, nil
	}
	threshInput := cleanedThresh
	maskedThresh := gocv.NewMat()
	defer maskedThresh.Close()
	if !innerROIMask.Empty() {
		gocv.BitwiseAnd(cleanedThresh, innerROIMask, &maskedThresh)
		threshInput = maskedThresh
	}

	contours := gocv.FindContours(threshInput, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()

	defects := d.extractDefectsFromContours(contours, targetW, targetH, "diff_contour")
	log.Printf(
		"detector.diff candidates stage=diff_contour count=%d broken_mode=%t",
		len(defects),
		brokenMode,
	)
	if brokenMode {
		beforeOverlap := len(defects)
		defects = d.filterDefectsByMaskOverlap(defects, structuralInput, d.BrokenMinOverlapRatio)
		afterOverlap := len(defects)
		defects = d.mergeNearbyDefects(defects, d.BrokenMergeDistance)
		afterMerge := len(defects)
		defects = d.keepDominantBrokenDefects(defects, d.BrokenDominantMinRatio)
		afterDominant := len(defects)
		log.Printf(
			"detector.diff broken_filter before_overlap=%d after_overlap=%d after_merge=%d after_dominant=%d",
			beforeOverlap,
			afterOverlap,
			afterMerge,
			afterDominant,
		)
		if len(defects) == 0 {
			defects = d.defectsFromMask(structuralInput, targetW, targetH, "broken_structural_mask")
			defects = d.mergeNearbyDefects(defects, d.BrokenMergeDistance)
			defects = d.keepDominantBrokenDefects(defects, d.BrokenDominantMinRatio)
			log.Printf("detector.diff broken_fallback stage=broken_structural_mask count=%d", len(defects))
		}
	}
	d.logDefects("inspect_diff", defects)

	return &entity.InspectionResult{
		ImageWidth:  targetW,
		ImageHeight: targetH,
		Defects:     defects,
		HasDefects:  len(defects) > 0,
	}, nil
}

// HighlightDefects рисует прямоугольники вокруг дефектов и возвращает новую картинку.
func (d *GoCVDetector) HighlightDefects(imageData []byte, result *entity.InspectionResult) ([]byte, error) {
	mat, err := decodeToMat(imageData)
	if err != nil {
		return nil, err
	}
	defer mat.Close()

	if mat.Empty() {
		return nil, errors.New("empty image")
	}

	green := color.RGBA{G: 255, A: 255}
	for _, defect := range result.Defects {
		rect := image.Rect(defect.X, defect.Y, defect.X+defect.Width, defect.Y+defect.Height)
		gocv.Rectangle(&mat, rect, green, 2)
	}

	img, err := mat.ToImage()
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// decodeToMat превращает байты изображения в gocv.Mat.
func decodeToMat(imageData []byte) (gocv.Mat, error) {
	mat, err := gocv.IMDecode(imageData, gocv.IMReadColor)
	if err == nil && !mat.Empty() {
		return mat, nil
	}
	if !mat.Empty() {
		mat.Close()
	}
	return gocv.NewMat(), errors.New("failed to decode image")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func normalizeKernelSize(value int, fallback int) int {
	if value < 1 {
		value = fallback
	}
	if value < 1 {
		value = 1
	}
	if value%2 == 0 {
		value++
	}
	return value
}

func maxFloat(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}
	maxValue := values[0]
	for i := 1; i < len(values); i++ {
		if values[i] > maxValue {
			maxValue = values[i]
		}
	}
	return maxValue
}

func (d *GoCVDetector) extractDefectsFromContours(contours gocv.PointsVector, imageWidth, imageHeight int, reasonTag string) []entity.DefectArea {
	if reasonTag == "" {
		reasonTag = "contour"
	}

	minRectArea := int(float64(imageWidth*imageHeight) * d.MinAreaRatio)
	minContourArea := float64(imageWidth*imageHeight) * d.MinContourAreaRatio

	droppedByRectArea := 0
	droppedByInvalidHeight := 0
	droppedByContourArea := 0
	droppedByAspect := 0
	droppedByFill := 0

	candidates := make([]entity.DefectArea, 0, contours.Size())
	for i := 0; i < contours.Size(); i++ {
		c := contours.At(i)
		rect := gocv.BoundingRect(c)
		rectArea := rect.Dx() * rect.Dy()
		if rectArea < minRectArea || rectArea <= 0 {
			droppedByRectArea++
			continue
		}
		if rect.Dy() == 0 {
			droppedByInvalidHeight++
			continue
		}

		contourArea := gocv.ContourArea(c)
		if contourArea < minContourArea {
			droppedByContourArea++
			continue
		}

		aspect := float64(rect.Dx()) / float64(rect.Dy())
		if aspect < d.MinAspectRatio || aspect > d.MaxAspectRatio {
			droppedByAspect++
			continue
		}

		fillRatio := contourArea / float64(rectArea)
		if fillRatio < d.MinFillRatio {
			droppedByFill++
			continue
		}

		reason := fmt.Sprintf(
			"%s contour_area=%.1f fill=%.3f aspect=%.3f",
			reasonTag,
			contourArea,
			fillRatio,
			aspect,
		)
		candidates = append(candidates, entity.DefectArea{
			X:      rect.Min.X,
			Y:      rect.Min.Y,
			Width:  rect.Dx(),
			Height: rect.Dy(),
			Area:   rectArea,
			Reason: reason,
		})
	}

	filtered := d.suppressDuplicateDefects(candidates)
	log.Printf(
		"detector.filter stage=%s contours=%d kept=%d dropped_rect=%d dropped_h=%d dropped_contour=%d dropped_aspect=%d dropped_fill=%d",
		reasonTag,
		contours.Size(),
		len(filtered),
		droppedByRectArea,
		droppedByInvalidHeight,
		droppedByContourArea,
		droppedByAspect,
		droppedByFill,
	)
	return filtered
}

func (d *GoCVDetector) suppressDuplicateDefects(defects []entity.DefectArea) []entity.DefectArea {
	if len(defects) < 2 {
		return defects
	}

	sort.Slice(defects, func(i, j int) bool {
		return defects[i].Area > defects[j].Area
	})

	kept := make([]entity.DefectArea, 0, len(defects))
	for _, candidate := range defects {
		drop := false
		for _, existing := range kept {
			iou := boxIoU(candidate, existing)
			containment := boxContainment(candidate, existing)
			if iou >= d.NMSIoUThreshold || containment >= d.NMSContainmentRatio {
				drop = true
				break
			}
		}
		if !drop {
			kept = append(kept, candidate)
		}
	}

	return kept
}

func boxIoU(a, b entity.DefectArea) float64 {
	inter := intersectionArea(a, b)
	if inter <= 0 {
		return 0
	}
	union := a.Area + b.Area - inter
	if union <= 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func boxContainment(a, b entity.DefectArea) float64 {
	inter := intersectionArea(a, b)
	if inter <= 0 {
		return 0
	}
	minArea := minInt(a.Area, b.Area)
	if minArea <= 0 {
		return 0
	}
	return float64(inter) / float64(minArea)
}

func intersectionArea(a, b entity.DefectArea) int {
	ax2 := a.X + a.Width
	ay2 := a.Y + a.Height
	bx2 := b.X + b.Width
	by2 := b.Y + b.Height

	ix1 := maxInt(a.X, b.X)
	iy1 := maxInt(a.Y, b.Y)
	ix2 := minInt(ax2, bx2)
	iy2 := minInt(ay2, by2)

	if ix2 <= ix1 || iy2 <= iy1 {
		return 0
	}
	return (ix2 - ix1) * (iy2 - iy1)
}

func (d *GoCVDetector) detectBrokenPartMask(baseMask, currentMask gocv.Mat) (bool, gocv.Mat) {
	if baseMask.Empty() || currentMask.Empty() || baseMask.Rows() != currentMask.Rows() || baseMask.Cols() != currentMask.Cols() {
		return false, gocv.NewMat()
	}

	imageArea := float64(baseMask.Rows() * baseMask.Cols())
	minComponentArea := imageArea * d.BrokenMinComponentRatio

	baseComponents, baseArea := significantMaskComponents(baseMask, minComponentArea)
	currentComponents, currentArea := significantMaskComponents(currentMask, minComponentArea)
	focusComponents := currentComponents
	if len(focusComponents) < 2 {
		splitMask := d.erodeMaskForSplit(currentMask)
		defer splitMask.Close()
		if !splitMask.Empty() && gocv.CountNonZero(splitMask) > 0 {
			erodedComponents, _ := significantMaskComponents(splitMask, minComponentArea*0.45)
			if len(erodedComponents) >= 2 {
				focusComponents = erodedComponents
			}
		}
	}

	componentSplit := len(baseComponents) <= 1 && hasSeparatedPart(focusComponents, d.BrokenSecondRelMin)
	areaLoss := false
	if baseArea > 0 && currentArea < baseArea {
		loss := (baseArea - currentArea) / baseArea
		areaLoss = loss >= d.BrokenAreaLossRatio
	}
	if !componentSplit && !areaLoss {
		return false, gocv.NewMat()
	}

	structural := gocv.NewMat()
	gocv.BitwiseXor(baseMask, currentMask, &structural)
	if structural.Empty() || gocv.CountNonZero(structural) == 0 {
		return true, structural
	}

	cleanedStructural := d.postProcessDiffMask(structural)
	structural.Close()

	if !componentSplit {
		return true, cleanedStructural
	}

	focus := gocv.NewMatWithSize(baseMask.Rows(), baseMask.Cols(), gocv.MatTypeCV8U)
	focusExpand := maxInt(1, d.BrokenFocusExpand)
	for i := 1; i < len(focusComponents); i++ {
		rect := expandRect(focusComponents[i].rect, focusExpand, baseMask.Cols(), baseMask.Rows())
		gocv.Rectangle(&focus, rect, color.RGBA{R: 255, G: 255, B: 255, A: 255}, -1)
	}

	if gocv.CountNonZero(focus) == 0 {
		focus.Close()
		return true, cleanedStructural
	}

	focusedStructural := gocv.NewMat()
	gocv.BitwiseAnd(cleanedStructural, focus, &focusedStructural)
	focus.Close()
	cleanedStructural.Close()
	if focusedStructural.Empty() || gocv.CountNonZero(focusedStructural) == 0 {
		focusedStructural.Close()
		return true, gocv.NewMat()
	}
	return true, focusedStructural
}

func (d *GoCVDetector) detectGeometryMismatch(baseMask, currentMask gocv.Mat) (bool, gocv.Mat, string) {
	if !d.EnableGeometryCheck {
		return false, gocv.NewMat(), ""
	}
	if baseMask.Empty() || currentMask.Empty() || baseMask.Rows() != currentMask.Rows() || baseMask.Cols() != currentMask.Cols() {
		return false, gocv.NewMat(), ""
	}

	baseContour, _, ok := largestContourCopy(baseMask)
	if !ok {
		return false, gocv.NewMat(), ""
	}
	defer baseContour.Close()
	currentContour, _, ok := largestContourCopy(currentMask)
	if !ok {
		return false, gocv.NewMat(), ""
	}
	defer currentContour.Close()

	baseShape := describeContourShape(
		baseContour,
		d.GeometryMinConcavity,
		d.GeometryRoundMinCircularity,
		d.GeometryPolygonMinCircularity,
		d.GeometryPolygonMinExtent,
	)
	currentShape := describeContourShape(
		currentContour,
		d.GeometryMinConcavity,
		d.GeometryRoundMinCircularity,
		d.GeometryPolygonMinCircularity,
		d.GeometryPolygonMinExtent,
	)
	shapeScore := gocv.MatchShapes(baseContour, currentContour, gocv.ContoursMatchI2, 0)
	concavityGap := absInt(baseShape.concavity - currentShape.concavity)
	vertexGap := absInt(baseShape.vertices - currentShape.vertices)
	circularityGap := math.Abs(baseShape.circularity - currentShape.circularity)

	familyMismatch := baseShape.family != shapeFamilyUnknown &&
		currentShape.family != shapeFamilyUnknown &&
		baseShape.family != currentShape.family
	mismatchByToothed := baseShape.family == shapeFamilyToothed &&
		currentShape.family == shapeFamilyToothed &&
		concavityGap >= d.GeometryMinConcavityGap
	mismatchByPolygon := baseShape.family == shapeFamilyPolygon &&
		currentShape.family == shapeFamilyPolygon &&
		baseShape.vertices > 0 &&
		currentShape.vertices > 0 &&
		baseShape.vertices <= 12 &&
		currentShape.vertices <= 12 &&
		vertexGap >= d.GeometryPolygonVertexGap
	mismatchByRound := baseShape.family == shapeFamilyRound &&
		currentShape.family == shapeFamilyRound &&
		circularityGap >= d.GeometryRoundMaxCircularityGap
	// MatchShapes оставляем только как дополнительный сигнал для зубчатых деталей.
	mismatchByToothedShape := baseShape.family == shapeFamilyToothed &&
		currentShape.family == shapeFamilyToothed &&
		concavityGap >= 1 &&
		shapeScore > d.GeometryMatchMaxScore

	mismatch := familyMismatch || mismatchByToothed || mismatchByPolygon || mismatchByRound || mismatchByToothedShape
	if !mismatch {
		log.Printf(
			"detector.geometry mismatch=false family_base=%s family_current=%s shape_score=%.4f concavity_base=%d concavity_current=%d vertices_base=%d vertices_current=%d circularity_base=%.4f circularity_current=%.4f extent_base=%.4f extent_current=%.4f",
			baseShape.family,
			currentShape.family,
			shapeScore,
			baseShape.concavity,
			currentShape.concavity,
			baseShape.vertices,
			currentShape.vertices,
			baseShape.circularity,
			currentShape.circularity,
			baseShape.extent,
			currentShape.extent,
		)
		return false, gocv.NewMat(), ""
	}

	reasonCode := "shape_family"
	switch {
	case mismatchByToothed:
		reasonCode = "tooth_count"
	case mismatchByPolygon:
		reasonCode = "polygon_vertices"
	case mismatchByRound:
		reasonCode = "round_profile"
	case mismatchByToothedShape:
		reasonCode = "toothed_shape"
	case familyMismatch:
		reasonCode = "shape_family"
	}
	reason := fmt.Sprintf(
		"geometry_mismatch reason=%s family_base=%s family_current=%s shape_score=%.4f concavity_base=%d concavity_current=%d vertices_base=%d vertices_current=%d circularity_base=%.4f circularity_current=%.4f extent_base=%.4f extent_current=%.4f",
		reasonCode,
		baseShape.family,
		currentShape.family,
		shapeScore,
		baseShape.concavity,
		currentShape.concavity,
		baseShape.vertices,
		currentShape.vertices,
		baseShape.circularity,
		currentShape.circularity,
		baseShape.extent,
		currentShape.extent,
	)

	shapeMask := gocv.NewMat()
	gocv.BitwiseXor(baseMask, currentMask, &shapeMask)
	if shapeMask.Empty() || gocv.CountNonZero(shapeMask) == 0 {
		return true, shapeMask, reason
	}

	cleanedShapeMask := d.postProcessDiffMask(shapeMask)
	shapeMask.Close()
	outerRingMask := d.buildOuterRingMask(baseMask, currentMask)
	defer outerRingMask.Close()
	if outerRingMask.Empty() || gocv.CountNonZero(outerRingMask) == 0 {
		log.Printf(
			"detector.geometry mismatch=true reason=%s family_base=%s family_current=%s",
			reasonCode,
			baseShape.family,
			currentShape.family,
		)
		return true, cleanedShapeMask, reason
	}

	focusedShapeMask := gocv.NewMat()
	gocv.BitwiseAnd(cleanedShapeMask, outerRingMask, &focusedShapeMask)
	cleanedShapeMask.Close()
	if focusedShapeMask.Empty() || gocv.CountNonZero(focusedShapeMask) == 0 {
		focusedShapeMask.Close()
		log.Printf(
			"detector.geometry mismatch=true reason=%s family_base=%s family_current=%s fallback=empty_focus",
			reasonCode,
			baseShape.family,
			currentShape.family,
		)
		return true, gocv.NewMat(), reason
	}

	log.Printf(
		"detector.geometry mismatch=true reason=%s family_base=%s family_current=%s",
		reasonCode,
		baseShape.family,
		currentShape.family,
	)
	return true, focusedShapeMask, reason
}

func largestContourCopy(mask gocv.Mat) (gocv.PointVector, float64, bool) {
	if mask.Empty() {
		return gocv.NewPointVector(), 0, false
	}
	contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()
	if contours.Size() == 0 {
		return gocv.NewPointVector(), 0, false
	}

	maxIdx := -1
	maxArea := 0.0
	for i := 0; i < contours.Size(); i++ {
		c := contours.At(i)
		area := gocv.ContourArea(c)
		if area > maxArea {
			maxArea = area
			maxIdx = i
		}
	}
	if maxIdx < 0 {
		return gocv.NewPointVector(), 0, false
	}

	pts := contours.At(maxIdx).ToPoints()
	if len(pts) < 5 {
		return gocv.NewPointVector(), maxArea, false
	}
	return gocv.NewPointVectorFromPoints(pts), maxArea, true
}

func countContourConcavity(contour gocv.PointVector) int {
	if contour.Size() < 8 {
		return 0
	}
	perimeter := gocv.ArcLength(contour, true)
	if perimeter <= 0 {
		return 0
	}
	approx := gocv.ApproxPolyDP(contour, 0.006*perimeter, true)
	defer approx.Close()
	if approx.Size() < 8 {
		return 0
	}

	hull := gocv.NewMat()
	defer hull.Close()
	gocv.ConvexHull(approx, &hull, false, false)
	if hull.Empty() || hull.Rows() < 3 {
		return 0
	}

	defects := gocv.NewMat()
	defer defects.Close()
	gocv.ConvexityDefects(approx, hull, &defects)
	if defects.Empty() {
		return 0
	}
	return defects.Rows()
}

func describeContourShape(
	contour gocv.PointVector,
	minConcavity int,
	roundMinCircularity float64,
	polygonMinCircularity float64,
	polygonMinExtent float64,
) shapeDescriptor {
	area := gocv.ContourArea(contour)
	perimeter := gocv.ArcLength(contour, true)
	concavity := countContourConcavity(contour)
	vertices := countContourVertices(contour)
	circularity := contourCircularity(area, perimeter)
	extent := contourExtent(contour, area)

	return shapeDescriptor{
		family: determineShapeFamily(
			concavity,
			vertices,
			circularity,
			extent,
			minConcavity,
			roundMinCircularity,
			polygonMinCircularity,
			polygonMinExtent,
		),
		concavity:   concavity,
		vertices:    vertices,
		circularity: circularity,
		extent:      extent,
		area:        area,
		perimeter:   perimeter,
	}
}

func countContourVertices(contour gocv.PointVector) int {
	if contour.Size() < 3 {
		return contour.Size()
	}
	perimeter := gocv.ArcLength(contour, true)
	if perimeter <= 0 {
		return contour.Size()
	}

	approx := gocv.ApproxPolyDP(contour, 0.015*perimeter, true)
	defer approx.Close()
	if approx.Size() < 3 {
		return contour.Size()
	}
	return approx.Size()
}

func contourCircularity(area, perimeter float64) float64 {
	if area <= 0 || perimeter <= 0 {
		return 0
	}
	return 4.0 * math.Pi * area / (perimeter * perimeter)
}

func contourExtent(contour gocv.PointVector, area float64) float64 {
	if area <= 0 {
		return 0
	}
	rect := gocv.BoundingRect(contour)
	rectArea := rect.Dx() * rect.Dy()
	if rectArea <= 0 {
		return 0
	}
	return area / float64(rectArea)
}

func determineShapeFamily(
	concavity int,
	vertices int,
	circularity float64,
	extent float64,
	minConcavity int,
	roundMinCircularity float64,
	polygonMinCircularity float64,
	polygonMinExtent float64,
) shapeFamily {
	if concavity >= minConcavity {
		return shapeFamilyToothed
	}
	if circularity >= roundMinCircularity {
		return shapeFamilyRound
	}
	if vertices >= 3 && vertices <= 12 && circularity >= polygonMinCircularity && extent >= polygonMinExtent {
		return shapeFamilyPolygon
	}
	return shapeFamilyUnknown
}

func (d *GoCVDetector) buildOuterRingMask(baseMask, currentMask gocv.Mat) gocv.Mat {
	if baseMask.Empty() || currentMask.Empty() {
		return gocv.NewMat()
	}
	unionMask := gocv.NewMat()
	gocv.BitwiseOr(baseMask, currentMask, &unionMask)
	if unionMask.Empty() || gocv.CountNonZero(unionMask) == 0 {
		return unionMask
	}

	kernelSize := normalizeKernelSize(d.GeometryRingKernel, 41)
	kernel := gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(kernelSize, kernelSize))
	defer kernel.Close()

	eroded := gocv.NewMat()
	gocv.Erode(unionMask, &eroded, kernel)
	if eroded.Empty() || gocv.CountNonZero(eroded) == 0 {
		eroded.Close()
		return unionMask
	}

	ring := gocv.NewMat()
	gocv.BitwiseXor(unionMask, eroded, &ring)
	unionMask.Close()
	eroded.Close()
	return ring
}

func significantMaskComponents(mask gocv.Mat, minArea float64) ([]maskComponent, float64) {
	if mask.Empty() {
		return nil, 0
	}
	contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()

	components := make([]maskComponent, 0, contours.Size())
	total := 0.0
	for i := 0; i < contours.Size(); i++ {
		c := contours.At(i)
		area := gocv.ContourArea(c)
		if area < minArea {
			continue
		}
		components = append(components, maskComponent{
			rect: gocv.BoundingRect(c),
			area: area,
		})
		total += area
	}

	sort.Slice(components, func(i, j int) bool {
		return components[i].area > components[j].area
	})

	return components, total
}

func hasSeparatedPart(components []maskComponent, minRelativeArea float64) bool {
	if len(components) < 2 {
		return false
	}
	mainArea := components[0].area
	if mainArea <= 0 {
		return false
	}
	secondArea := components[1].area
	return secondArea/mainArea >= minRelativeArea
}

func expandRect(rect image.Rectangle, margin int, maxW int, maxH int) image.Rectangle {
	expanded := image.Rect(rect.Min.X-margin, rect.Min.Y-margin, rect.Max.X+margin, rect.Max.Y+margin)
	bounds := image.Rect(0, 0, maxW, maxH)
	return expanded.Intersect(bounds)
}

func (d *GoCVDetector) filterDefectsByMaskOverlap(defects []entity.DefectArea, mask gocv.Mat, minOverlap float64) []entity.DefectArea {
	if len(defects) == 0 || mask.Empty() || gocv.CountNonZero(mask) == 0 {
		return defects
	}
	filtered := make([]entity.DefectArea, 0, len(defects))
	for idx, defect := range defects {
		overlap := overlapRatioForDefect(defect, mask)
		if overlap >= minOverlap {
			defect.Reason = appendReason(defect.Reason, fmt.Sprintf("broken_overlap=%.3f", overlap))
			filtered = append(filtered, defect)
			continue
		}
		log.Printf(
			"detector.reject stage=broken_overlap idx=%d overlap=%.3f min=%.3f bbox=(x=%d y=%d w=%d h=%d)",
			idx,
			overlap,
			minOverlap,
			defect.X,
			defect.Y,
			defect.Width,
			defect.Height,
		)
	}
	return filtered
}

func overlapRatioForDefect(defect entity.DefectArea, mask gocv.Mat) float64 {
	if mask.Empty() || defect.Width <= 0 || defect.Height <= 0 {
		return 0
	}
	rect := image.Rect(defect.X, defect.Y, defect.X+defect.Width, defect.Y+defect.Height)
	bounds := image.Rect(0, 0, mask.Cols(), mask.Rows())
	rect = rect.Intersect(bounds)
	if rect.Empty() {
		return 0
	}
	region := mask.Region(rect)
	defer region.Close()
	maskPixels := gocv.CountNonZero(region)
	area := rect.Dx() * rect.Dy()
	if area <= 0 {
		return 0
	}
	return float64(maskPixels) / float64(area)
}

func (d *GoCVDetector) mergeNearbyDefects(defects []entity.DefectArea, distance int) []entity.DefectArea {
	if len(defects) < 2 {
		return defects
	}
	if distance < 0 {
		distance = 0
	}

	merged := make([]entity.DefectArea, 0, len(defects))
	for _, current := range defects {
		wasMerged := false
		for i := range merged {
			if shouldMergeDefects(merged[i], current, distance) {
				merged[i] = unionDefectAreas(merged[i], current)
				wasMerged = true
				break
			}
		}
		if !wasMerged {
			merged = append(merged, current)
		}
	}

	if len(merged) == len(defects) {
		return merged
	}
	return d.mergeNearbyDefects(merged, distance)
}

func (d *GoCVDetector) keepDominantBrokenDefects(defects []entity.DefectArea, minRatio float64) []entity.DefectArea {
	if len(defects) < 2 {
		return defects
	}
	if minRatio <= 0 {
		return defects
	}
	if minRatio > 1 {
		minRatio = 1
	}

	maxArea := 0
	for _, defect := range defects {
		if defect.Area > maxArea {
			maxArea = defect.Area
		}
	}
	if maxArea <= 0 {
		return defects
	}

	kept := make([]entity.DefectArea, 0, len(defects))
	for idx, defect := range defects {
		ratio := float64(defect.Area) / float64(maxArea)
		if ratio >= minRatio {
			defect.Reason = appendReason(defect.Reason, fmt.Sprintf("broken_dominant=%.3f", ratio))
			kept = append(kept, defect)
			continue
		}
		log.Printf(
			"detector.reject stage=broken_dominant idx=%d ratio=%.3f min=%.3f bbox=(x=%d y=%d w=%d h=%d area=%d)",
			idx,
			ratio,
			minRatio,
			defect.X,
			defect.Y,
			defect.Width,
			defect.Height,
			defect.Area,
		)
	}

	if len(kept) == 0 {
		return defects
	}
	return kept
}

func shouldMergeDefects(a, b entity.DefectArea, distance int) bool {
	if boxIoU(a, b) > 0 {
		return true
	}
	ax1, ay1 := a.X, a.Y
	ax2, ay2 := a.X+a.Width, a.Y+a.Height
	bx1, by1 := b.X, b.Y
	bx2, by2 := b.X+b.Width, b.Y+b.Height

	dx := maxInt(0, maxInt(ax1, bx1)-minInt(ax2, bx2))
	dy := maxInt(0, maxInt(ay1, by1)-minInt(ay2, by2))
	return dx <= distance && dy <= distance
}

func unionDefectAreas(a, b entity.DefectArea) entity.DefectArea {
	x1 := minInt(a.X, b.X)
	y1 := minInt(a.Y, b.Y)
	x2 := maxInt(a.X+a.Width, b.X+b.Width)
	y2 := maxInt(a.Y+a.Height, b.Y+b.Height)
	return entity.DefectArea{
		X:      x1,
		Y:      y1,
		Width:  x2 - x1,
		Height: y2 - y1,
		Area:   (x2 - x1) * (y2 - y1),
		Reason: combineReasons(a.Reason, b.Reason),
	}
}

func (d *GoCVDetector) defectsFromMask(mask gocv.Mat, imageWidth int, imageHeight int, reasonTag string) []entity.DefectArea {
	if mask.Empty() || gocv.CountNonZero(mask) == 0 {
		return nil
	}
	contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()
	return d.extractDefectsFromContours(contours, imageWidth, imageHeight, reasonTag)
}

func (d *GoCVDetector) buildGeometryMismatchDefects(mask gocv.Mat, imageWidth int, imageHeight int, reason string) []entity.DefectArea {
	if mask.Empty() || gocv.CountNonZero(mask) == 0 {
		return nil
	}

	imageArea := float64(imageWidth * imageHeight)
	minComponentArea := imageArea * d.MinContourAreaRatio
	rect, ok := unionRectForMask(mask, minComponentArea)
	if !ok || rect.Dx() <= 0 || rect.Dy() <= 0 {
		return nil
	}

	return []entity.DefectArea{
		{
			X:      rect.Min.X,
			Y:      rect.Min.Y,
			Width:  rect.Dx(),
			Height: rect.Dy(),
			Area:   rect.Dx() * rect.Dy(),
			Reason: appendReason(reason, "geometry_mask_union"),
		},
	}
}

func unionRectForMask(mask gocv.Mat, minArea float64) (image.Rectangle, bool) {
	if mask.Empty() {
		return image.Rectangle{}, false
	}
	contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()
	if contours.Size() == 0 {
		return image.Rectangle{}, false
	}

	var merged image.Rectangle
	hasRect := false
	for i := 0; i < contours.Size(); i++ {
		c := contours.At(i)
		if gocv.ContourArea(c) < minArea {
			continue
		}
		rect := gocv.BoundingRect(c)
		if rect.Dx() <= 0 || rect.Dy() <= 0 {
			continue
		}
		if !hasRect {
			merged = rect
			hasRect = true
			continue
		}
		merged = unionRect(merged, rect)
	}
	return merged, hasRect
}

func unionRect(a, b image.Rectangle) image.Rectangle {
	x1 := minInt(a.Min.X, b.Min.X)
	y1 := minInt(a.Min.Y, b.Min.Y)
	x2 := maxInt(a.Max.X, b.Max.X)
	y2 := maxInt(a.Max.Y, b.Max.Y)
	return image.Rect(x1, y1, x2, y2)
}

func (d *GoCVDetector) erodeMaskForSplit(mask gocv.Mat) gocv.Mat {
	if mask.Empty() {
		return gocv.NewMat()
	}
	kernelSize := normalizeKernelSize(d.BrokenSplitKernel, 17)
	kernel := gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(kernelSize, kernelSize))
	defer kernel.Close()
	eroded := gocv.NewMat()
	gocv.Erode(mask, &eroded, kernel)
	if eroded.Empty() || gocv.CountNonZero(eroded) == 0 {
		eroded.Close()
		return gocv.NewMat()
	}
	return eroded
}

func (d *GoCVDetector) checkImageQuality(mat gocv.Mat, label string, glareLimit float64) error {
	if mat.Empty() {
		return fmt.Errorf("quality gate failed for %s: empty image", label)
	}

	if mat.Cols() < d.MinImageSide || mat.Rows() < d.MinImageSide {
		return fmt.Errorf("quality gate failed for %s: image is too small (%dx%d)", label, mat.Cols(), mat.Rows())
	}

	gray := gocv.NewMat()
	defer gray.Close()
	gocv.CvtColor(mat, &gray, gocv.ColorBGRToGray)

	// Считаем метрики качества только по области детали, чтобы фон не давал ложные отказы.
	partMask := d.buildPartMask(mat)
	defer partMask.Close()
	qualityMask := d.buildInteriorMask(partMask)
	defer qualityMask.Close()
	if qualityMask.Empty() || gocv.CountNonZero(qualityMask) == 0 {
		return fmt.Errorf("quality gate failed for %s: part ROI is empty", label)
	}
	roiArea := gocv.CountNonZero(qualityMask)
	totalArea := mat.Rows() * mat.Cols()
	roiRatio := 1.0
	if totalArea > 0 {
		roiRatio = float64(roiArea) / float64(totalArea)
	}
	// Если ROI слишком большой, значит маска детали ненадёжна (часто весь кадр).
	// В этом режиме не блокируем кадр по фотометрии, чтобы не ловить ложные отказы на белом фоне.
	relaxedPhotometricGate := roiRatio > 0.90

	edges := gocv.NewMat()
	defer edges.Close()
	gocv.Canny(gray, &edges, 80, 160)
	edgeRatio := ratioOfMaskInROI(edges, qualityMask)
	minEdgeRatio := d.MinSharpnessEdgeRatio
	if relaxedPhotometricGate {
		minEdgeRatio *= 0.35
	}
	if edgeRatio < minEdgeRatio {
		return fmt.Errorf("quality gate failed for %s: image is blurry (edge_ratio=%.4f)", label, edgeRatio)
	}

	bright := gocv.NewMat()
	defer bright.Close()
	gocv.Threshold(gray, &bright, 250, 255, gocv.ThresholdBinary)
	overexposedRatio := ratioOfMaskInROI(bright, qualityMask)
	if !relaxedPhotometricGate && overexposedRatio > d.MaxOverexposedRatio {
		return fmt.Errorf("quality gate failed for %s: overexposed image (ratio=%.4f)", label, overexposedRatio)
	}

	dark := gocv.NewMat()
	defer dark.Close()
	gocv.Threshold(gray, &dark, 20, 255, gocv.ThresholdBinaryInv)
	underexposedRatio := ratioOfMaskInROI(dark, qualityMask)
	if !relaxedPhotometricGate && underexposedRatio > d.MaxUnderexposedRatio {
		return fmt.Errorf("quality gate failed for %s: underexposed image (ratio=%.4f)", label, underexposedRatio)
	}

	hsv := gocv.NewMat()
	defer hsv.Close()
	gocv.CvtColor(mat, &hsv, gocv.ColorBGRToHSV)
	channels := gocv.Split(hsv)
	for i := range channels {
		defer channels[i].Close()
	}
	if len(channels) < 3 {
		return fmt.Errorf("quality gate failed for %s: invalid hsv channels", label)
	}

	lowSat := gocv.NewMat()
	defer lowSat.Close()
	gocv.Threshold(channels[1], &lowSat, 40, 255, gocv.ThresholdBinaryInv)

	highVal := gocv.NewMat()
	defer highVal.Close()
	gocv.Threshold(channels[2], &highVal, 245, 255, gocv.ThresholdBinary)

	glare := gocv.NewMat()
	defer glare.Close()
	gocv.BitwiseAnd(lowSat, highVal, &glare)
	glareRatio := ratioOfMaskInROI(glare, qualityMask)
	if !relaxedPhotometricGate && glareRatio > glareLimit {
		return fmt.Errorf("quality gate failed for %s: too much glare (ratio=%.4f)", label, glareRatio)
	}

	return nil
}

func ratioOfMask(mask gocv.Mat) float64 {
	total := mask.Cols() * mask.Rows()
	if total <= 0 {
		return 0
	}
	return float64(gocv.CountNonZero(mask)) / float64(total)
}

func ratioOfMaskInROI(mask, roi gocv.Mat) float64 {
	if mask.Empty() || roi.Empty() || mask.Rows() != roi.Rows() || mask.Cols() != roi.Cols() {
		return ratioOfMask(mask)
	}

	roiArea := gocv.CountNonZero(roi)
	if roiArea == 0 {
		return ratioOfMask(mask)
	}

	masked := gocv.NewMat()
	defer masked.Close()
	gocv.BitwiseAnd(mask, roi, &masked)
	return float64(gocv.CountNonZero(masked)) / float64(roiArea)
}

func (d *GoCVDetector) buildPartMask(mat gocv.Mat) gocv.Mat {
	if mat.Empty() {
		return gocv.NewMat()
	}

	gray := gocv.NewMat()
	defer gray.Close()
	gocv.CvtColor(mat, &gray, gocv.ColorBGRToGray)

	blur := gocv.NewMat()
	defer blur.Close()
	gocv.GaussianBlur(gray, &blur, image.Pt(5, 5), 0, 0, gocv.BorderDefault)

	edges := gocv.NewMat()
	defer edges.Close()
	gocv.Canny(blur, &edges, 40, 120)

	kernelDilate := gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(5, 5))
	defer kernelDilate.Close()
	expanded := gocv.NewMat()
	defer expanded.Close()
	gocv.Dilate(edges, &expanded, kernelDilate)

	contours := gocv.FindContours(expanded, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()

	mask := gocv.NewMatWithSize(mat.Rows(), mat.Cols(), gocv.MatTypeCV8U)
	if contours.Size() == 0 {
		mask.SetTo(gocv.NewScalar(255, 255, 255, 0))
		return mask
	}

	maxIdx := -1
	maxArea := 0.0
	areas := make([]float64, contours.Size())
	for i := 0; i < contours.Size(); i++ {
		c := contours.At(i)
		area := gocv.ContourArea(c)
		areas[i] = area
		if area > maxArea {
			maxArea = area
			maxIdx = i
		}
	}

	totalArea := float64(mat.Rows() * mat.Cols())
	if maxIdx < 0 || totalArea <= 0 || maxArea/totalArea < d.MinPartAreaRatio {
		mask.SetTo(gocv.NewScalar(255, 255, 255, 0))
		return mask
	}

	gocv.DrawContours(&mask, contours, maxIdx, color.RGBA{R: 255, G: 255, B: 255, A: 255}, -1)

	minSecondaryArea := maxFloat(
		float64(mat.Rows()*mat.Cols())*d.PartSecondaryAreaRatio,
		maxArea*d.PartSecondaryRelRatio,
	)
	for i := 0; i < contours.Size(); i++ {
		if i == maxIdx {
			continue
		}
		if areas[i] < minSecondaryArea {
			continue
		}
		gocv.DrawContours(&mask, contours, i, color.RGBA{R: 255, G: 255, B: 255, A: 255}, -1)
	}

	kernelClose := gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(9, 9))
	defer kernelClose.Close()
	gocv.MorphologyEx(mask, &mask, gocv.MorphClose, kernelClose)
	return mask
}

func (d *GoCVDetector) buildInteriorMask(mask gocv.Mat) gocv.Mat {
	if mask.Empty() {
		return gocv.NewMat()
	}
	if d.ROIMarginKernel < 3 || d.ROIMarginKernel%2 == 0 {
		return mask.Clone()
	}

	kernel := gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(d.ROIMarginKernel, d.ROIMarginKernel))
	defer kernel.Close()
	inner := gocv.NewMat()
	gocv.Erode(mask, &inner, kernel)
	if inner.Empty() || gocv.CountNonZero(inner) == 0 {
		inner.Close()
		return mask.Clone()
	}
	return inner
}

func (d *GoCVDetector) postProcessDiffMask(thresh gocv.Mat) gocv.Mat {
	if thresh.Empty() {
		return gocv.NewMat()
	}

	openKernelSize := normalizeKernelSize(d.DiffOpenKernel, 3)
	closeKernelSize := normalizeKernelSize(d.DiffCloseKernel, 5)

	openKernel := gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(openKernelSize, openKernelSize))
	defer openKernel.Close()
	closeKernel := gocv.GetStructuringElement(gocv.MorphEllipse, image.Pt(closeKernelSize, closeKernelSize))
	defer closeKernel.Close()

	opened := gocv.NewMat()
	gocv.MorphologyEx(thresh, &opened, gocv.MorphOpen, openKernel)

	cleaned := gocv.NewMat()
	gocv.MorphologyEx(opened, &cleaned, gocv.MorphClose, closeKernel)
	opened.Close()

	return cleaned
}

func (d *GoCVDetector) alignCurrentToBase(baseMat, currentMat, baseMask, currentMask gocv.Mat) (gocv.Mat, gocv.Mat, float64, error) {
	baseRect, ok := largestMaskRect(baseMask)
	if !ok {
		return gocv.NewMat(), gocv.NewMat(), 0, errors.New("alignment failed: base mask is not detected")
	}
	currentRect, ok := largestMaskRect(currentMask)
	if !ok {
		return gocv.NewMat(), gocv.NewMat(), 0, errors.New("alignment failed: current mask is not detected")
	}
	if baseRect.Dx() <= 0 || baseRect.Dy() <= 0 || currentRect.Dx() <= 0 || currentRect.Dy() <= 0 {
		return gocv.NewMat(), gocv.NewMat(), 0, errors.New("alignment failed: invalid mask rectangles")
	}

	scaleX := float64(baseRect.Dx()) / float64(currentRect.Dx())
	scaleY := float64(baseRect.Dy()) / float64(currentRect.Dy())
	if scaleX <= 0 || scaleY <= 0 {
		return gocv.NewMat(), gocv.NewMat(), 0, errors.New("alignment failed: invalid scale")
	}

	resizedW := maxInt(1, int(float64(currentMat.Cols())*scaleX))
	resizedH := maxInt(1, int(float64(currentMat.Rows())*scaleY))

	resizedCurrent := gocv.NewMat()
	defer resizedCurrent.Close()
	gocv.Resize(currentMat, &resizedCurrent, image.Pt(resizedW, resizedH), 0, 0, gocv.InterpolationLinear)

	resizedMask := gocv.NewMat()
	defer resizedMask.Close()
	gocv.Resize(currentMask, &resizedMask, image.Pt(resizedW, resizedH), 0, 0, gocv.InterpolationNearestNeighbor)

	scaledCurrentRect := image.Rect(
		int(float64(currentRect.Min.X)*scaleX),
		int(float64(currentRect.Min.Y)*scaleY),
		int(float64(currentRect.Max.X)*scaleX),
		int(float64(currentRect.Max.Y)*scaleY),
	)

	baseCenterX := baseRect.Min.X + baseRect.Dx()/2
	baseCenterY := baseRect.Min.Y + baseRect.Dy()/2
	currentCenterX := scaledCurrentRect.Min.X + scaledCurrentRect.Dx()/2
	currentCenterY := scaledCurrentRect.Min.Y + scaledCurrentRect.Dy()/2

	offsetX := baseCenterX - currentCenterX
	offsetY := baseCenterY - currentCenterY

	alignedCurrent := gocv.NewMatWithSize(baseMat.Rows(), baseMat.Cols(), currentMat.Type())
	alignedCurrent.SetTo(gocv.NewScalar(0, 0, 0, 0))
	alignedMask := gocv.NewMatWithSize(baseMat.Rows(), baseMat.Cols(), gocv.MatTypeCV8U)
	alignedMask.SetTo(gocv.NewScalar(0, 0, 0, 0))

	srcRect := image.Rect(0, 0, resizedW, resizedH)
	dstRect := image.Rect(offsetX, offsetY, offsetX+resizedW, offsetY+resizedH)
	bounds := image.Rect(0, 0, baseMat.Cols(), baseMat.Rows())
	clippedDst := dstRect.Intersect(bounds)
	if clippedDst.Empty() {
		alignedCurrent.Close()
		alignedMask.Close()
		return gocv.NewMat(), gocv.NewMat(), 0, errors.New("alignment failed: no overlap after transform")
	}

	shiftX := clippedDst.Min.X - dstRect.Min.X
	shiftY := clippedDst.Min.Y - dstRect.Min.Y
	clippedSrc := image.Rect(
		srcRect.Min.X+shiftX,
		srcRect.Min.Y+shiftY,
		srcRect.Min.X+shiftX+clippedDst.Dx(),
		srcRect.Min.Y+shiftY+clippedDst.Dy(),
	)
	if clippedSrc.Empty() {
		alignedCurrent.Close()
		alignedMask.Close()
		return gocv.NewMat(), gocv.NewMat(), 0, errors.New("alignment failed: empty source overlap")
	}

	srcCurrentROI := resizedCurrent.Region(clippedSrc)
	defer srcCurrentROI.Close()
	dstCurrentROI := alignedCurrent.Region(clippedDst)
	defer dstCurrentROI.Close()
	srcCurrentROI.CopyTo(&dstCurrentROI)

	srcMaskROI := resizedMask.Region(clippedSrc)
	defer srcMaskROI.Close()
	dstMaskROI := alignedMask.Region(clippedDst)
	defer dstMaskROI.Close()
	srcMaskROI.CopyTo(&dstMaskROI)

	score := maskIoU(baseMask, alignedMask)
	refinedCurrent, refinedMask, refinedScore, ok := d.refineAlignmentECC(baseMat, alignedCurrent, baseMask, alignedMask)
	if ok {
		if refinedScore > score {
			alignedCurrent.Close()
			alignedMask.Close()
			return refinedCurrent, refinedMask, refinedScore, nil
		}
		refinedCurrent.Close()
		refinedMask.Close()
	}

	return alignedCurrent, alignedMask, score, nil
}

func (d *GoCVDetector) refineAlignmentECC(baseMat, roughCurrent, baseMask, roughMask gocv.Mat) (gocv.Mat, gocv.Mat, float64, bool) {
	if baseMat.Empty() || roughCurrent.Empty() || baseMask.Empty() || roughMask.Empty() {
		return gocv.NewMat(), gocv.NewMat(), 0, false
	}

	baseGray := gocv.NewMat()
	defer baseGray.Close()
	gocv.CvtColor(baseMat, &baseGray, gocv.ColorBGRToGray)

	currentGray := gocv.NewMat()
	defer currentGray.Close()
	gocv.CvtColor(roughCurrent, &currentGray, gocv.ColorBGRToGray)

	eccMask := d.buildInteriorMask(baseMask)
	defer eccMask.Close()
	if eccMask.Empty() || gocv.CountNonZero(eccMask) == 0 {
		return gocv.NewMat(), gocv.NewMat(), 0, false
	}

	warp := gocv.Eye(2, 3, gocv.MatTypeCV32F)
	defer warp.Close()
	criteria := gocv.NewTermCriteria(gocv.Count+gocv.EPS, 80, 1e-4)

	ecc := gocv.FindTransformECC(baseGray, currentGray, &warp, gocv.MotionAffine, criteria, eccMask, 5)
	if ecc <= 0 {
		return gocv.NewMat(), gocv.NewMat(), 0, false
	}

	refinedCurrent := gocv.NewMat()
	gocv.WarpAffineWithParams(
		roughCurrent,
		&refinedCurrent,
		warp,
		image.Pt(baseMat.Cols(), baseMat.Rows()),
		gocv.InterpolationLinear+gocv.WarpInverseMap,
		gocv.BorderConstant,
		color.RGBA{},
	)
	if refinedCurrent.Empty() {
		refinedCurrent.Close()
		return gocv.NewMat(), gocv.NewMat(), 0, false
	}

	refinedMask := gocv.NewMat()
	gocv.WarpAffineWithParams(
		roughMask,
		&refinedMask,
		warp,
		image.Pt(baseMat.Cols(), baseMat.Rows()),
		gocv.InterpolationNearestNeighbor+gocv.WarpInverseMap,
		gocv.BorderConstant,
		color.RGBA{},
	)
	if refinedMask.Empty() || gocv.CountNonZero(refinedMask) == 0 {
		refinedCurrent.Close()
		refinedMask.Close()
		return gocv.NewMat(), gocv.NewMat(), 0, false
	}

	score := maskIoU(baseMask, refinedMask)
	return refinedCurrent, refinedMask, score, true
}

func largestMaskRect(mask gocv.Mat) (image.Rectangle, bool) {
	if mask.Empty() {
		return image.Rectangle{}, false
	}
	contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()
	if contours.Size() == 0 {
		return image.Rectangle{}, false
	}

	maxIdx := -1
	maxArea := 0.0
	for i := 0; i < contours.Size(); i++ {
		c := contours.At(i)
		area := gocv.ContourArea(c)
		if area > maxArea {
			maxArea = area
			maxIdx = i
		}
	}
	if maxIdx < 0 {
		return image.Rectangle{}, false
	}
	return gocv.BoundingRect(contours.At(maxIdx)), true
}

func maskIoU(a, b gocv.Mat) float64 {
	if a.Empty() || b.Empty() || a.Rows() != b.Rows() || a.Cols() != b.Cols() {
		return 0
	}
	inter := gocv.NewMat()
	defer inter.Close()
	union := gocv.NewMat()
	defer union.Close()
	gocv.BitwiseAnd(a, b, &inter)
	gocv.BitwiseOr(a, b, &union)
	den := gocv.CountNonZero(union)
	if den == 0 {
		return 0
	}
	return float64(gocv.CountNonZero(inter)) / float64(den)
}

func appendReason(baseReason, extra string) string {
	if extra == "" {
		return baseReason
	}
	if baseReason == "" {
		return extra
	}
	return baseReason + "; " + extra
}

func combineReasons(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if a == b {
		return a
	}
	return a + " | " + b
}

func (d *GoCVDetector) logDefects(stage string, defects []entity.DefectArea) {
	if len(defects) == 0 {
		log.Printf("detector.defects stage=%s count=0", stage)
		return
	}
	for i, defect := range defects {
		reason := defect.Reason
		if reason == "" {
			reason = "not_set"
		}
		log.Printf(
			"detector.defect stage=%s idx=%d bbox=(x=%d y=%d w=%d h=%d area=%d) reason=%s",
			stage,
			i,
			defect.X,
			defect.Y,
			defect.Width,
			defect.Height,
			defect.Area,
			reason,
		)
	}
}
