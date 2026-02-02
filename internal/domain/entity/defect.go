package entity

// DefectArea представляет область с обнаруженным дефектом
type DefectArea struct {
	X      int // координата X левого верхнего угла
	Y      int // координата Y левого верхнего угла
	Width  int // ширина области в пикселях
	Height int // высота области в пикселях
	Area   int // площадь области в пикселях
}

// Center возвращает координаты центра дефекта
func (d DefectArea) Center() (x, y int) {
	return d.X + d.Width/2, d.Y + d.Height/2
}
