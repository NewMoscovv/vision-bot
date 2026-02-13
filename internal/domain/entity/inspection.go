package entity

// InspectionResult хранит итог анализа изображения.
type InspectionResult struct {
	ImageWidth  int          // ширина изображения
	ImageHeight int          // высота изображения
	Defects     []DefectArea // список найденных дефектов
	HasDefects  bool         // флаг наличия дефектов
}

// AiDescription — текстовое описание дефектов от ИИ.
type AiDescription struct {
	Text string
}
