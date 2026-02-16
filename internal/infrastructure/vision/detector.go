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

	"gocv.io/x/gocv"

	"vision-bot/internal/domain/entity"
)

type GoCVDetector struct {
	MinAreaRatio          float64
	MaxAspectRatio        float64
	MinAspectRatio        float64
	MaxSide               int
	MinImageSide          int
	MinSharpnessEdgeRatio float64
	MaxOverexposedRatio   float64
	MaxUnderexposedRatio  float64
	MaxGlareRatio         float64
	DiffMaxGlareRatio     float64
	MinPartAreaRatio      float64
	ROIMarginKernel       int
	EnableRegistration    bool
	MinAlignmentScore     float64
}

// NewGoCVDetector создаёт детектор с минимальной площадью дефекта.
func NewGoCVDetector(minArea int) *GoCVDetector {
	return &GoCVDetector{
		MinAreaRatio:          0.001,
		MinAspectRatio:        0.1,
		MaxAspectRatio:        10.0,
		MaxSide:               1024,
		MinImageSide:          400,
		MinSharpnessEdgeRatio: 0.008,
		MaxOverexposedRatio:   0.35,
		MaxUnderexposedRatio:  0.45,
		MaxGlareRatio:         0.20,
		DiffMaxGlareRatio:     0.26,
		MinPartAreaRatio:      0.05,
		ROIMarginKernel:       9,
		EnableRegistration:    true,
		MinAlignmentScore:     0.25,
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

	minArea := int(float64(mat.Cols()*mat.Rows()) * d.MinAreaRatio)
	defects := make([]entity.DefectArea, 0, contours.Size())
	for i := 0; i < contours.Size(); i++ {
		c := contours.At(i)
		rect := gocv.BoundingRect(c)
		area := rect.Dx() * rect.Dy()
		if area < minArea {
			continue
		}

		if rect.Dy() == 0 {
			continue
		}
		aspect := float64(rect.Dx()) / float64(rect.Dy())
		if aspect < d.MinAspectRatio || aspect > d.MaxAspectRatio {
			continue
		}
		defects = append(defects, entity.DefectArea{
			X:      rect.Min.X,
			Y:      rect.Min.Y,
			Width:  rect.Dx(),
			Height: rect.Dy(),
			Area:   area,
		})
	}

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
	gocv.Threshold(blur, &thresh, 25, 255, gocv.ThresholdBinary)
	threshInput := thresh
	maskedThresh := gocv.NewMat()
	defer maskedThresh.Close()
	if !innerROIMask.Empty() {
		gocv.BitwiseAnd(thresh, innerROIMask, &maskedThresh)
		threshInput = maskedThresh
	}

	contours := gocv.FindContours(threshInput, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()

	minArea := int(float64(targetW*targetH) * d.MinAreaRatio)
	defects := make([]entity.DefectArea, 0, contours.Size())
	for i := 0; i < contours.Size(); i++ {
		c := contours.At(i)
		rect := gocv.BoundingRect(c)
		area := rect.Dx() * rect.Dy()
		if area < minArea {
			continue
		}

		if rect.Dy() == 0 {
			continue
		}
		aspect := float64(rect.Dx()) / float64(rect.Dy())
		if aspect < d.MinAspectRatio || aspect > d.MaxAspectRatio {
			continue
		}

		defects = append(defects, entity.DefectArea{
			X:      rect.Min.X,
			Y:      rect.Min.Y,
			Width:  rect.Dx(),
			Height: rect.Dy(),
			Area:   area,
		})
	}

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
	for i := 0; i < contours.Size(); i++ {
		c := contours.At(i)
		area := gocv.ContourArea(c)
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
