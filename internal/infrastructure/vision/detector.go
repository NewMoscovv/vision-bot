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
		MaxGlareRatio:         0.08,
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
	if err := d.checkImageQuality(mat, "image"); err != nil {
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

	blur := gocv.NewMat()
	defer blur.Close()
	gocv.GaussianBlur(gray, &blur, image.Pt(5, 5), 0, 0, gocv.BorderDefault)

	edges := gocv.NewMat()
	defer edges.Close()
	gocv.Canny(blur, &edges, 50, 150)

	contours := gocv.FindContours(edges, gocv.RetrievalExternal, gocv.ChainApproxSimple)
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
	if err := d.checkImageQuality(baseMat, "base image"); err != nil {
		return nil, err
	}
	if err := d.checkImageQuality(currentMat, "current image"); err != nil {
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

	// Переводим в серый и считаем абсолютную разницу.
	baseGray := gocv.NewMat()
	defer baseGray.Close()
	gocv.CvtColor(baseMat, &baseGray, gocv.ColorBGRToGray)

	currentGray := gocv.NewMat()
	defer currentGray.Close()
	gocv.CvtColor(currentMat, &currentGray, gocv.ColorBGRToGray)

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

	contours := gocv.FindContours(thresh, gocv.RetrievalExternal, gocv.ChainApproxSimple)
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

func (d *GoCVDetector) checkImageQuality(mat gocv.Mat, label string) error {
	if mat.Empty() {
		return fmt.Errorf("quality gate failed for %s: empty image", label)
	}

	if mat.Cols() < d.MinImageSide || mat.Rows() < d.MinImageSide {
		return fmt.Errorf("quality gate failed for %s: image is too small (%dx%d)", label, mat.Cols(), mat.Rows())
	}

	gray := gocv.NewMat()
	defer gray.Close()
	gocv.CvtColor(mat, &gray, gocv.ColorBGRToGray)

	edges := gocv.NewMat()
	defer edges.Close()
	gocv.Canny(gray, &edges, 80, 160)
	edgeRatio := ratioOfMask(edges)
	if edgeRatio < d.MinSharpnessEdgeRatio {
		return fmt.Errorf("quality gate failed for %s: image is blurry (edge_ratio=%.4f)", label, edgeRatio)
	}

	bright := gocv.NewMat()
	defer bright.Close()
	gocv.Threshold(gray, &bright, 250, 255, gocv.ThresholdBinary)
	overexposedRatio := ratioOfMask(bright)
	if overexposedRatio > d.MaxOverexposedRatio {
		return fmt.Errorf("quality gate failed for %s: overexposed image (ratio=%.4f)", label, overexposedRatio)
	}

	dark := gocv.NewMat()
	defer dark.Close()
	gocv.Threshold(gray, &dark, 20, 255, gocv.ThresholdBinaryInv)
	underexposedRatio := ratioOfMask(dark)
	if underexposedRatio > d.MaxUnderexposedRatio {
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
	glareRatio := ratioOfMask(glare)
	if glareRatio > d.MaxGlareRatio {
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
