# Техническое решение: стабильная система детекции дефектов детали

## 1. Цель
Построить промышленно пригодный pipeline контроля качества детали по фото, где дефектами считаются:
- трещины (`crack`)
- царапины (`scratch`)
- повреждения зубцов (`tooth_damage`)
- надрезы/сколы (`notch_chip`)
- поломанные части детали (`broken_part`)

Система должна возвращать мало ложных срабатываний при реальных вариациях съёмки (свет, сдвиг, масштаб, пересжатие).

## 2. Почему текущий подход нестабилен
Текущий подход на уровне `AbsDiff + Threshold + Contours` корректен как прототип, но не как production:
- нет геометрического выравнивания эталона и текущего снимка
- нет компенсации освещения/бликов для металлической поверхности
- одна общая логика для разных типов дефектов
- нет объединения дублей кандидатов (NMS/merge)
- критерии решений жёстко прошиты и не профилируются под тип детали

Итог: большое число ложных дефектов на краях, бликах и текстуре поверхности.

## 3. Архитектурные принципы
1. Детекция должна быть stage-based (модульный pipeline), а не монолитной функцией.
2. Для геометрических и поверхностных дефектов используются разные ветки алгоритмов.
3. Все пороги и правила выносятся в версионируемые профили, а не хардкодятся.
4. Каждый релиз проходит регрессию на размеченном golden-наборе.
5. Результат должен быть объясним: `что`, `где`, `почему`, `насколько уверенно`.

## 4. Целевой pipeline инспекции

### 4.1 Stage 0: Input Quality Gate
Цель: не анализировать заведомо плохие снимки.

Проверки:
- резкость (variance of Laplacian)
- пересвет/недосвет (доля насыщенных пикселей)
- сильный блик (glare ratio)
- минимальный размер входа

Если качество ниже порога, система возвращает `RETAKE_REQUIRED` с причиной.

### 4.2 Stage 1: Локализация детали и маска
Цель: убрать фон, тени стола, лишние объекты.

Результат:
- бинарная маска детали
- канонический bounding box детали

### 4.3 Stage 2: Геометрическое выравнивание (Registration)
Цель: привести проверяемое изображение к системе координат эталона.

Метод:
- ORB/SIFT признаки + RANSAC -> affine/homography
- fallback: ECC alignment

Порог по качеству выравнивания обязателен. При плохом `alignment_score` — retake.

### 4.4 Stage 3: Нормализация освещения
Цель: подавить влияние бликов и неравномерного света.

Рекомендуется:
- CLAHE для локального контраста
- top-hat/black-hat для подавления крупномасштабных градиентов
- опционально Retinex

### 4.5 Stage 4A: Ветка геометрических дефектов
Для `tooth_damage`, `notch_chip`, `broken_part`.

Алгоритм:
- сравнение масок/контуров эталона и текущего изображения в выровненной системе
- расстояние контуров (Hausdorff/Chamfer-приближение)
- выделение областей «недостающий материал / лишний материал»

Ключевые признаки кандидата:
- площадь, периметр
- локализация (например, зона зубцов)
- дефицит/избыток формы относительно эталона

### 4.6 Stage 4B: Ветка поверхностных дефектов
Для `crack`, `scratch`.

Алгоритм:
- карта высокочастотных отличий после нормализации
- multi-scale line/ridge detection (Frangi/Sato или аналог)
- connected components и скелетизация

Ключевые признаки:
- длина, средняя ширина, эксцентриситет
- извилистость/линейность
- контраст вдоль нормали к линии
- устойчивость на нескольких масштабах

### 4.7 Stage 5: Классификация кандидатов
Кандидаты из обеих веток классифицируются в один из классов дефектов.

Рекомендуемый путь:
- baseline: правила + градиентный бустинг по handcrafted-features
- target: patch-level CNN/ViT классификатор поверх кандидатов

### 4.8 Stage 6: Дедупликация и объединение
- NMS по IoU
- graph merge для пересекающихся/смежных компонент
- агрегация в один инстанс дефекта

### 4.9 Stage 7: Decision Layer
Финальные правила качества:
- пороги по каждому типу дефекта (разные)
- критичные типы (`broken_part`) -> reject без допусловий
- итоговый verdict + severity

## 5. Выходной контракт (предлагаемый)

```json
{
  "image_width": 1280,
  "image_height": 853,
  "verdict": "REJECT",
  "has_defects": true,
  "defects": [
    {
      "id": "d1",
      "type": "crack",
      "score": 0.93,
      "severity": "high",
      "bbox": {"x": 365, "y": 403, "w": 55, "h": 68},
      "polygon": [[...]],
      "features": {
        "length_px": 63.4,
        "width_px": 2.9,
        "elongation": 11.2
      },
      "reason_code": "SURFACE_LINE_HIGH_CONTRAST"
    }
  ],
  "diagnostics": {
    "quality_gate": "PASS",
    "alignment_score": 0.91,
    "profile_version": "wrench_v2.1",
    "timings_ms": {
      "preprocess": 37,
      "geometry_branch": 24,
      "surface_branch": 41,
      "postprocess": 9
    }
  }
}
```

## 6. Идиоматичная структура модулей в проекте

Предлагаемая структура (расширение текущей Clean Architecture):

```text
internal/
  domain/
    entity/
      defect.go
      inspection.go
      diagnostics.go
      defect_type.go
    port/
      detector.go
      quality_gate.go
      profiler.go

  application/
    inspection_pipeline.go      # оркестрация стадий
    decision_service.go         # правила verdict/severity

  infrastructure/
    vision/
      pipeline/
        inspector.go            # основной pipeline
      preprocess/
        quality_gate.go
        segmenter.go
        aligner.go
        normalize.go
      detectors/
        geometry_detector.go
        surface_detector.go
      classify/
        candidate_classifier.go
      postprocess/
        nms.go
        merger.go
      profiles/
        wrench_v2.1.yaml
```

## 7. Интерфейсы стадий (уровень доменных портов)

```go
type QualityGate interface {
    Check(img gocv.Mat) (QualityReport, error)
}

type Aligner interface {
    Align(base, current gocv.Mat, mask gocv.Mat) (aligned gocv.Mat, score float64, err error)
}

type CandidateDetector interface {
    Detect(base, current gocv.Mat, mask gocv.Mat) ([]Candidate, error)
}

type CandidateClassifier interface {
    Classify([]Candidate) ([]Defect, error)
}

type PostProcessor interface {
    Merge([]Defect) ([]Defect, error)
}
```

Важно: зависимости направлены из application в domain, а реализации лежат в infrastructure.

## 8. Конфигурация и профили
Каждый тип детали должен иметь профиль:
- `part_type`
- параметры quality gate
- параметры alignment
- пороги ветки геометрии
- пороги ветки поверхности
- правила decision layer

Формат: YAML в `infrastructure/vision/profiles/`.

Пример секций профиля:
- `quality.min_sharpness`
- `align.min_score`
- `surface.min_crack_length`
- `surface.max_crack_width`
- `geometry.max_missing_area_ratio`
- `decision.reject_rules`

## 9. Данные и валидация
Без датасета pipeline будет нестабилен.

Нужно:
- train/val/test набор, разметка по 5 классам дефектов
- негативные примеры без дефектов в реальных условиях
- версия датасета и freeze тестового набора

Ключевые метрики:
- per-class precision/recall/F1
- false positives per image
- missed critical defects
- стабильность по условиям света/ракурса

## 10. Регрессионный контур
Обязательный CI-gate:
- запуск инспекции на golden-наборе
- сравнение метрик с baseline
- блокировка merge при деградации выше порога

## 11. План внедрения (итеративно)

### Этап 1: Стабилизация текущего CV baseline
- добавить quality gate
- добавить registration
- добавить ROI mask детали
- заменить фильтрацию contour-кандидатов на shape-features
- добавить NMS/merge

### Этап 2: Две специализированные ветки
- geometry detector для зубцов/сколов/поломок
- surface detector для трещин/царапин
- единая fusion-логика

### Этап 3: ML-классификация кандидатов
- ввести классификатор по candidate patches
- калибровка score

### Этап 4: Production-hardening
- профили на типы деталей
- регрессионные гейты в CI
- диагностика и аналитика качества

## 12. Ожидаемый результат
После внедрения данной структуры система:
- уменьшает ложные срабатывания на бликах и границах
- стабильно находит реальные дефекты разных типов
- масштабируется на новые детали через профиль, а не через правки кода
- обеспечивает повторяемое качество за счёт регрессии и метрик

## 13. Декомпозиция задач (backlog)

### Epic A: Контракты и каркас (P0)
Задачи:
- добавить перечисление типов дефектов: `crack`, `scratch`, `tooth_damage`, `notch_chip`, `broken_part`
- расширить `InspectionResult`: `verdict`, `defects[].type`, `defects[].score`, `defects[].severity`, `reason_code`, `diagnostics`
- обновить порт детектора под расширенный контракт
- добавить загрузчик профилей и базовый профиль `wrench_v2.1`

Definition of Done:
- проект собирается с новым контрактом
- unit-тесты на сериализацию/валидацию результата проходят
- текущий flow бота не ломается

### Epic B: Стабилизация baseline (P0)
Задачи:
- реализовать `Input Quality Gate` (blur/exposure/glare/min-size)
- реализовать маску детали и ROI-анализ
- добавить выравнивание `base/current` (ORB + RANSAC, fallback ECC)
- заменить фильтрацию кандидатов по `BoundingRect` на shape-признаки
- добавить `NMS/merge` для дедупликации кандидатов

Definition of Done:
- на кейсе с одной трещиной возвращается один инстанс дефекта
- количество ложных срабатываний по golden-набору заметно снижено
- в диагностике сохраняются `alignment_score` и причины отбраковки

### Epic C: Специализированные ветки детекции (P1)
Задачи:
- геометрическая ветка для `tooth_damage`, `notch_chip`, `broken_part`
- поверхностная ветка для `crack`, `scratch`
- fusion-слой объединения кандидатов двух веток

Definition of Done:
- каждая ветка имеет собственные тесты на позитивные и негативные примеры
- итоговый результат содержит единый список дефектов с типами

### Epic D: Decision Layer и правила брака (P1)
Задачи:
- настроить per-class пороги и логику `PASS/WARN/REJECT`
- выделить критичные дефекты (`broken_part`) в отдельные reject-правила
- добавить explainability: `reason_code` и краткое объяснение решения

Definition of Done:
- правила решения читаются из профиля
- итоговый verdict воспроизводим на одинаковых входных данных

### Epic E: Датасет, метрики, регрессия (P0)
Задачи:
- зафиксировать формат разметки и структуру датасета
- собрать golden-набор: норма + 5 классов дефектов
- реализовать regression-runner и отчёт метрик
- добавить CI quality-gate по метрикам

Definition of Done:
- есть baseline метрик (`precision`, `recall`, `F1`, `FP/image`)
- merge блокируется при деградации выше согласованного порога

### Epic F: Интеграция в Telegram flow (P1)
Задачи:
- добавить сценарий `RETAKE_REQUIRED` с причиной
- обновить ответы бота: verdict + типы дефектов + подсветка
- закрыть e2e-сценарии для пользовательских состояний и ошибок

Definition of Done:
- пользователь получает понятный и устойчивый результат в одном сообщении
- обработаны невалидные входы и ошибки инфраструктуры

## 14. Рекомендуемый порядок выполнения
1. Epic A -> контракты и профиль.
2. Epic B -> снять главные ложные срабатывания.
3. Epic E -> зафиксировать качество и не деградировать.
4. Epic C -> добавить полноту по всем типам дефектов.
5. Epic D -> формализовать правила брака.
6. Epic F -> довести UX до production-уровня.
