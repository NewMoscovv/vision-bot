# Архитектура проекта Vision Bot

## 1. Обзор

**Vision Bot** — Telegram-бот для автоматического поиска дефектов на фотографиях деталей с использованием компьютерного зрения и генерации текстовых описаний через ИИ.

### Основной поток данных

```
┌─────────────┐    ┌─────────────┐    ┌─────────────────┐    ┌─────────────┐
│  Telegram   │───▶│ Bot Handler │───▶│ DefectDetector  │───▶│  Describer  │
│   (фото)    │    │             │    │    (GoCV)       │    │   (Qwen)    │
└─────────────┘    └─────────────┘    └─────────────────┘    └─────────────┘
                          │                    │                    │
                          │                    ▼                    ▼
                          │           ┌─────────────────┐   ┌─────────────┐
                          │           │ Изображение с   │   │  Текстовое  │
                          │           │  подсветкой     │   │  описание   │
                          │           └─────────────────┘   └─────────────┘
                          │                    │                    │
                          ◀────────────────────┴────────────────────┘
                          │
                          ▼
                   ┌─────────────┐
                   │  Ответ в    │
                   │  Telegram   │
                   └─────────────┘
```

---

## 2. Технический стек

| Компонент | Технология | Версия |
|-----------|------------|--------|
| Язык программирования | Go | 1.21+ |
| Telegram Bot API | go-telegram-bot-api | v5.5.1 |
| Компьютерное зрение | GoCV (OpenCV) | v0.36+ / OpenCV 4.9 |
| ИИ-модель | Qwen2.5 через Ollama | 7B / 14B |
| Конфигурация | caarlos0/env + godotenv | v10 / v1.5 |
| Логирование | uber-go/zap | v1.27 |
| Контейнеризация | Docker | - |

### Зависимости (go.mod)

```go
module vision-bot

go 1.21

require (
    // Telegram Bot
    github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
    
    // Computer Vision
    gocv.io/x/gocv v0.36.1
    
    // Конфигурация
    github.com/caarlos0/env/v10 v10.0.0
    github.com/joho/godotenv v1.5.1
    
    // Логирование
    go.uber.org/zap v1.27.0
    
    // Graceful shutdown
    golang.org/x/sync v0.6.0
)
```

---

## 3. Архитектура (Clean Architecture)

Проект построен по принципам Clean Architecture с разделением на слои:

```
┌────────────────────────────────────────────────────────────────┐
│                        API LAYER (api/)                        │
│                 ┌──────────────────────────┐                   │
│                 │   Telegram Bot Handler   │                   │
│                 │    (точка входа)         │                   │
│                 └────────────┬─────────────┘                   │
└──────────────────────────────┼─────────────────────────────────┘
                               │ uses
                               ▼
┌────────────────────────────────────────────────────────────────┐
│                     APPLICATION LAYER                          │
│              ┌───────────────────────────────┐                 │
│              │     InspectionService         │                 │
│              │   (оркестрация use-case)      │                 │
│              └───────────────┬───────────────┘                 │
└──────────────────────────────┼─────────────────────────────────┘
                               │ uses
                               ▼
┌────────────────────────────────────────────────────────────────┐
│                      DOMAIN LAYER (ports)                      │
│  ┌──────────────────────────┐  ┌────────────────────────────┐  │
│  │    DefectDetector        │  │     DefectDescriber        │  │
│  │      interface           │  │        interface           │  │
│  └──────────────────────────┘  └────────────────────────────┘  │
│                                                                │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐  │
│  │ DefectArea   │  │ Inspection   │  │   AiDescription      │  │
│  │   entity     │  │   Result     │  │      entity          │  │
│  └──────────────┘  └──────────────┘  └──────────────────────┘  │
└────────────────────────────────────────────────────────────────┘
          ▲                               ▲
          │ implements                    │ implements
          │                               │
┌─────────┴───────────────────────────────┴─────────────────────┐
│                    INFRASTRUCTURE LAYER                       │
│       ┌──────────────────┐       ┌──────────────────────┐     │
│       │   GoCV Detector  │       │   Ollama Describer   │     │
│       │  (vision/)       │       │      (ai/)           │     │
│       └──────────────────┘       └──────────────────────┘     │
└───────────────────────────────────────────────────────────────┘
```

### Принцип зависимостей

- **API Layer** (`api/`) — точка входа, знает об Application Layer
- **Application Layer** — оркестрация, знает только о Domain Layer (интерфейсах)
- **Domain Layer** — ядро, не зависит ни от чего внешнего
- **Infrastructure Layer** — реализации, зависит от Domain Layer (реализует интерфейсы)

---

## 4. Структура проекта

```
vision-bot/
├── cmd/
│   └── bot/
│       └── main.go                 # Точка входа, DI, запуск
│
├── api/                            # Транспортный слой (точки входа)
│   └── telegram/
│       ├── bot.go                  # Инициализация и запуск бота
│       ├── handler.go              # Обработчики /start, /help, фото
│       └── sender.go               # Формирование и отправка ответов
│
├── internal/
│   ├── domain/                     # Доменный слой
│   │   ├── entity/
│   │   │   ├── defect.go           # DefectArea
│   │   │   ├── inspection.go       # InspectionResult, AiDescription
│   │   │   └── user.go             # User, UserState
│   │   │
│   │   └── port/                   # Интерфейсы (порты)
│   │       ├── detector.go         # DefectDetector interface
│   │       ├── describer.go        # DefectDescriber interface
│   │       └── user_repository.go  # UserRepository interface
│   │
│   ├── application/                # Application слой
│   │   └── inspection/
│   │       └── service.go          # InspectionService
│   │
│   └── infrastructure/             # Инфраструктурный слой
│       ├── vision/
│       │   ├── detector.go         # GoCV реализация
│       │   ├── preprocessor.go     # Предобработка изображений
│       │   └── highlighter.go      # Подсветка дефектов
│       │
│       ├── ai/
│       │   ├── ollama.go           # Ollama/Qwen реализация
│       │   └── prompt.go           # Системные промпты
│       │
│       └── storage/
│           ├── temp.go                    # Временное хранение файлов
│           └── memory_user_repository.go  # In-memory хранилище пользователей
│
├── config/                         # Конфигурация приложения
│   └── config.go                   # Структура и загрузка конфига
│
├── pkg/                            # Общие утилиты
│   └── imgutil/
│       └── converter.go            # Конвертация изображений
│
├── docs/
│   ├── тех задание.md
│   └── architecture.md             # Этот документ
│
├── .env.example
├── .gitignore
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
├── docker-compose.yml
└── README.md
```

---

## 5. Доменные сущности

### DefectArea

```go
// internal/domain/entity/defect.go
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
```

### InspectionResult

```go
// internal/domain/entity/inspection.go
package entity

// InspectionResult результат инспекции изображения
type InspectionResult struct {
    ImageWidth  int          // ширина изображения
    ImageHeight int          // высота изображения
    Defects     []DefectArea // список найденных дефектов
    HasDefects  bool         // флаг наличия дефектов
}

// AiDescription текстовое описание дефектов от ИИ
type AiDescription struct {
    Text string
}
```

### User (Пользователь)

Пользователь — субъект, взаимодействующий с ботом через Telegram. Для каждого пользователя отслеживается текущее состояние диалога, что позволяет реализовать многошаговые сценарии взаимодействия.

```go
// internal/domain/entity/user.go
package entity

// UserState состояние пользователя в диалоге
type UserState string

const (
    StateMainMenu      UserState = "main_menu"       // В главном меню
    StateAwaitingPhoto UserState = "awaiting_photo"  // Ожидание фото детали
    StateProcessing    UserState = "processing"      // Обработка изображения
)

// User представляет пользователя бота
type User struct {
    ID     int64     // Telegram User ID
    ChatID int64     // Telegram Chat ID
    State  UserState // Текущее состояние пользователя
}

// NewUser создаёт нового пользователя с начальным состоянием
func NewUser(userID, chatID int64) *User {
    return &User{
        ID:     userID,
        ChatID: chatID,
        State:  StateMainMenu,
    }
}

// SetState обновляет состояние пользователя
func (u *User) SetState(state UserState) {
    u.State = state
}
```

#### Диаграмма состояний

```
┌─────────────┐
│  MainMenu   │◄─────────────────────────────────┐
└──────┬──────┘                                  │
       │ /start, /check                          │
       ▼                                         │
┌──────────────┐                                 │
│AwaitingPhoto │                                 │
└──────┬───────┘                                 │
       │ получено фото                           │
       ▼                                         │
┌─────────────┐      результат отправлен         │
│ Processing  │──────────────────────────────────┘
└─────────────┘
```

#### Переходы состояний

| Текущее состояние | Событие | Новое состояние | Действие |
|-------------------|---------|-----------------|----------|
| MainMenu | /start | MainMenu | Приветствие |
| MainMenu | /check | AwaitingPhoto | Запрос фото |
| MainMenu | фото | Processing | Начать обработку |
| AwaitingPhoto | фото | Processing | Начать обработку |
| AwaitingPhoto | /cancel | MainMenu | Отмена |
| Processing | обработка завершена | MainMenu | Отправить результат |
| * | текст (не команда) | — | Подсказка отправить фото |

#### Интерфейс репозитория

```go
// internal/domain/port/user_repository.go
package port

import (
    "context"
    "vision-bot/internal/domain/entity"
)

// UserRepository интерфейс хранилища пользователей
type UserRepository interface {
    // Get возвращает пользователя по ID, создаёт нового если не найден
    Get(ctx context.Context, userID, chatID int64) (*entity.User, error)
    
    // Save сохраняет состояние пользователя
    Save(ctx context.Context, user *entity.User) error
    
    // UpdateState обновляет состояние пользователя
    UpdateState(ctx context.Context, userID int64, state entity.UserState) error
}
```

#### In-Memory реализация (для прототипа)

```go
// internal/infrastructure/storage/memory_user_repository.go
package storage

import (
    "context"
    "sync"
    "vision-bot/internal/domain/entity"
    "vision-bot/internal/domain/port"
)

// MemoryUserRepository in-memory хранилище пользователей
type MemoryUserRepository struct {
    mu    sync.RWMutex
    users map[int64]*entity.User
}

// NewMemoryUserRepository создаёт новое in-memory хранилище
func NewMemoryUserRepository() *MemoryUserRepository {
    return &MemoryUserRepository{
        users: make(map[int64]*entity.User),
    }
}

func (r *MemoryUserRepository) Get(ctx context.Context, userID, chatID int64) (*entity.User, error) {
    r.mu.RLock()
    user, exists := r.users[userID]
    r.mu.RUnlock()
    
    if exists {
        return user, nil
    }
    
    // Создаём нового пользователя
    newUser := entity.NewUser(userID, chatID)
    r.mu.Lock()
    r.users[userID] = newUser
    r.mu.Unlock()
    
    return newUser, nil
}

func (r *MemoryUserRepository) Save(ctx context.Context, user *entity.User) error {
    r.mu.Lock()
    r.users[user.ID] = user
    r.mu.Unlock()
    return nil
}

func (r *MemoryUserRepository) UpdateState(ctx context.Context, userID int64, state entity.UserState) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    
    if user, exists := r.users[userID]; exists {
        user.SetState(state)
    }
    return nil
}

var _ port.UserRepository = (*MemoryUserRepository)(nil)
```

#### Примечания

- Для прототипа используется in-memory хранение (данные теряются при перезапуске).
- При перезапуске бота все пользователи получают состояние `MainMenu`.
- Потокобезопасность обеспечивается через `sync.RWMutex`.
- В будущем можно легко заменить на Redis/PostgreSQL реализацию благодаря интерфейсу.

---

## 6. Интерфейсы (Ports)

### DefectDetector

```go
// internal/domain/port/detector.go
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
```

### DefectDescriber

```go
// internal/domain/port/describer.go
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
```

---

## 7. Application Service

```go
// internal/application/inspection/service.go
package inspection

import (
    "context"
    "vision-bot/internal/domain/entity"
    "vision-bot/internal/domain/port"
)

// Service оркестрирует процесс инспекции
type Service struct {
    detector  port.DefectDetector
    describer port.DefectDescriber
}

// NewService создаёт новый сервис инспекции
func NewService(detector port.DefectDetector, describer port.DefectDescriber) *Service {
    return &Service{
        detector:  detector,
        describer: describer,
    }
}

// InspectionOutput результат инспекции для отправки пользователю
type InspectionOutput struct {
    HasDefects       bool
    Description      string
    HighlightedImage []byte // nil если дефектов нет
}

// Inspect выполняет полный цикл инспекции
func (s *Service) Inspect(ctx context.Context, imageData []byte) (*InspectionOutput, error) {
    // 1. Детекция дефектов
    result, err := s.detector.Inspect(ctx, imageData)
    if err != nil {
        return nil, err
    }
    
    // 2. Если дефектов нет — возвращаем простой результат
    if !result.HasDefects {
        return &InspectionOutput{
            HasDefects:  false,
            Description: "Дефекты не обнаружены.",
        }, nil
    }
    
    // 3. Подсветка дефектов на изображении
    highlighted, err := s.detector.HighlightDefects(imageData, result)
    if err != nil {
        return nil, err
    }
    
    // 4. Генерация текстового описания через ИИ
    aiDesc, err := s.describer.Describe(ctx, result)
    if err != nil {
        // Fallback: возвращаем результат без описания ИИ
        return &InspectionOutput{
            HasDefects:       true,
            Description:      "Обнаружены дефекты, но не удалось получить подробное описание.",
            HighlightedImage: highlighted,
        }, nil
    }
    
    return &InspectionOutput{
        HasDefects:       true,
        Description:      aiDesc.Text,
        HighlightedImage: highlighted,
    }, nil
}
```

---

## 8. ИИ-модуль (Ollama + Qwen)

### Конфигурация Ollama

```go
// internal/infrastructure/ai/ollama.go
package ai

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "vision-bot/internal/domain/entity"
    "vision-bot/internal/domain/port"
)

type OllamaDescriber struct {
    baseURL string
    model   string
    client  *http.Client
}

func NewOllamaDescriber(baseURL, model string) *OllamaDescriber {
    return &OllamaDescriber{
        baseURL: baseURL,  // http://localhost:11434
        model:   model,    // qwen2.5:7b
        client:  &http.Client{},
    }
}

type ollamaRequest struct {
    Model  string `json:"model"`
    Prompt string `json:"prompt"`
    Stream bool   `json:"stream"`
}

type ollamaResponse struct {
    Response string `json:"response"`
}

func (o *OllamaDescriber) Describe(ctx context.Context, result *entity.InspectionResult) (*entity.AiDescription, error) {
    prompt := buildPrompt(result)
    
    reqBody := ollamaRequest{
        Model:  o.model,
        Prompt: prompt,
        Stream: false,
    }
    
    jsonBody, _ := json.Marshal(reqBody)
    
    req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/generate", bytes.NewBuffer(jsonBody))
    if err != nil {
        return nil, err
    }
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := o.client.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var ollamaResp ollamaResponse
    if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
        return nil, err
    }
    
    return &entity.AiDescription{Text: ollamaResp.Response}, nil
}

// Проверка реализации интерфейса
var _ port.DefectDescriber = (*OllamaDescriber)(nil)
```

### Системный промпт

```go
// internal/infrastructure/ai/prompt.go
package ai

import (
    "fmt"
    "strings"
    "vision-bot/internal/domain/entity"
)

const systemPrompt = `Ты инженер по контролю качества деталей. 
Тебе передают размеры изображения и список дефектных областей с координатами.
Твоя задача — кратко описать найденные дефекты на русском языке:
- сколько дефектов обнаружено;
- где они расположены (верх/низ, слева/справа относительно центра);
- какие из них крупнее по площади.
Ответ должен быть 2-4 предложения, понятных человеку.`

func buildPrompt(result *entity.InspectionResult) string {
    var sb strings.Builder
    
    sb.WriteString(systemPrompt)
    sb.WriteString("\n\n")
    sb.WriteString(fmt.Sprintf("Размер изображения: %d x %d пикселей\n", result.ImageWidth, result.ImageHeight))
    sb.WriteString(fmt.Sprintf("Количество дефектов: %d\n\n", len(result.Defects)))
    
    for i, d := range result.Defects {
        sb.WriteString(fmt.Sprintf("Дефект %d: позиция (%d, %d), размер %dx%d, площадь %d пикселей\n",
            i+1, d.X, d.Y, d.Width, d.Height, d.Area))
    }
    
    sb.WriteString("\nОпиши эти дефекты кратко и понятно:")
    
    return sb.String()
}
```

---

## 9. Модуль компьютерного зрения (GoCV)

### Детектор дефектов

```go
// internal/infrastructure/vision/detector.go
package vision

import (
    "context"
    "image/color"
    
    "gocv.io/x/gocv"
    "vision-bot/internal/domain/entity"
    "vision-bot/internal/domain/port"
)

type GoCVDetector struct {
    minArea     float64 // минимальная площадь дефекта
    blurSize    int     // размер размытия
    threshold   float64 // порог бинаризации
}

func NewGoCVDetector(minArea float64, blurSize int, threshold float64) *GoCVDetector {
    return &GoCVDetector{
        minArea:   minArea,   // например, 100.0
        blurSize:  blurSize,  // например, 5
        threshold: threshold, // например, 30.0
    }
}

func (d *GoCVDetector) Inspect(ctx context.Context, imageData []byte) (*entity.InspectionResult, error) {
    // 1. Декодирование изображения
    img, err := gocv.IMDecode(imageData, gocv.IMReadColor)
    if err != nil {
        return nil, err
    }
    defer img.Close()
    
    // 2. Преобразование в оттенки серого
    gray := gocv.NewMat()
    defer gray.Close()
    gocv.CvtColor(img, &gray, gocv.ColorBGRToGray)
    
    // 3. Размытие для уменьшения шума
    blurred := gocv.NewMat()
    defer blurred.Close()
    gocv.GaussianBlur(gray, &blurred, image.Point{d.blurSize, d.blurSize}, 0, 0, gocv.BorderDefault)
    
    // 4. Выделение краёв (детекция аномалий)
    edges := gocv.NewMat()
    defer edges.Close()
    gocv.Canny(blurred, &edges, d.threshold, d.threshold*2)
    
    // 5. Морфологические операции для улучшения контуров
    kernel := gocv.GetStructuringElement(gocv.MorphRect, image.Point{3, 3})
    defer kernel.Close()
    gocv.Dilate(edges, &edges, kernel)
    
    // 6. Поиск контуров
    contours := gocv.FindContours(edges, gocv.RetrievalExternal, gocv.ChainApproxSimple)
    defer contours.Close()
    
    // 7. Фильтрация и формирование результата
    var defects []entity.DefectArea
    for i := 0; i < contours.Size(); i++ {
        contour := contours.At(i)
        area := gocv.ContourArea(contour)
        
        if area >= d.minArea {
            rect := gocv.BoundingRect(contour)
            defects = append(defects, entity.DefectArea{
                X:      rect.Min.X,
                Y:      rect.Min.Y,
                Width:  rect.Dx(),
                Height: rect.Dy(),
                Area:   int(area),
            })
        }
    }
    
    return &entity.InspectionResult{
        ImageWidth:  img.Cols(),
        ImageHeight: img.Rows(),
        Defects:     defects,
        HasDefects:  len(defects) > 0,
    }, nil
}

func (d *GoCVDetector) HighlightDefects(imageData []byte, result *entity.InspectionResult) ([]byte, error) {
    img, err := gocv.IMDecode(imageData, gocv.IMReadColor)
    if err != nil {
        return nil, err
    }
    defer img.Close()
    
    // Рисуем прямоугольники вокруг дефектов
    red := color.RGBA{255, 0, 0, 255}
    for _, defect := range result.Defects {
        rect := image.Rect(defect.X, defect.Y, defect.X+defect.Width, defect.Y+defect.Height)
        gocv.Rectangle(&img, rect, red, 2)
    }
    
    // Кодируем обратно в JPEG
    buf, err := gocv.IMEncode(gocv.JPEGFileExt, img)
    if err != nil {
        return nil, err
    }
    defer buf.Close()
    
    return buf.GetBytes(), nil
}

var _ port.DefectDetector = (*GoCVDetector)(nil)
```

---

## 10. Конфигурация

```go
// config/config.go
package config

import (
    "github.com/caarlos0/env/v10"
    "github.com/joho/godotenv"
)

type Config struct {
    // Telegram
    TelegramToken string `env:"TELEGRAM_TOKEN,required"`
    
    // Ollama
    OllamaURL   string `env:"OLLAMA_URL" envDefault:"http://localhost:11434"`
    OllamaModel string `env:"OLLAMA_MODEL" envDefault:"qwen2.5:7b"`
    
    // Vision
    MinDefectArea float64 `env:"MIN_DEFECT_AREA" envDefault:"100"`
    BlurSize      int     `env:"BLUR_SIZE" envDefault:"5"`
    Threshold     float64 `env:"THRESHOLD" envDefault:"30"`
    
    // App
    LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
}

func Load() (*Config, error) {
    _ = godotenv.Load() // игнорируем ошибку если .env нет
    
    cfg := &Config{}
    if err := env.Parse(cfg); err != nil {
        return nil, err
    }
    return cfg, nil
}
```

### .env.example

```env
# Telegram Bot
TELEGRAM_TOKEN=your_bot_token_here

# Ollama (AI)
OLLAMA_URL=http://localhost:11434
OLLAMA_MODEL=qwen2.5:7b

# Vision parameters
MIN_DEFECT_AREA=100
BLUR_SIZE=5
THRESHOLD=30

# Logging
LOG_LEVEL=info
```

---

## 11. Docker

### Dockerfile

```dockerfile
# Build stage
FROM gocv/opencv:4.9.0 AS builder

WORKDIR /app

# Копируем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем приложение
RUN CGO_ENABLED=1 GOOS=linux go build -o /bot ./cmd/bot

# Runtime stage
FROM gocv/opencv:4.9.0-static

WORKDIR /app

# Копируем бинарник
COPY --from=builder /bot /app/bot

# Запуск
CMD ["/app/bot"]
```

### docker-compose.yml

```yaml
version: '3.8'

services:
  ollama:
    image: ollama/ollama:latest
    container_name: vision-bot-ollama
    ports:
      - "11434:11434"
    volumes:
      - ollama_data:/root/.ollama
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 1
              capabilities: [gpu]
    # Для CPU-only убрать секцию deploy

  bot:
    build: .
    container_name: vision-bot
    depends_on:
      - ollama
    environment:
      - TELEGRAM_TOKEN=${TELEGRAM_TOKEN}
      - OLLAMA_URL=http://ollama:11434
      - OLLAMA_MODEL=qwen2.5:7b
    restart: unless-stopped

volumes:
  ollama_data:
```

---

## 12. Makefile

```makefile
.PHONY: build run test lint docker-build docker-up docker-down ollama-pull

# Сборка
build:
	go build -o bin/bot ./cmd/bot

# Запуск локально
run: build
	./bin/bot

# Тесты
test:
	go test -v ./...

# Линтер
lint:
	golangci-lint run

# Docker
docker-build:
	docker-compose build

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

# Загрузка модели Qwen в Ollama
ollama-pull:
	ollama pull qwen2.5:7b
```

---

## 13. Запуск проекта

### Локальный запуск

```bash
# 1. Установить Ollama
curl -fsSL https://ollama.com/install.sh | sh

# 2. Загрузить модель Qwen
ollama pull qwen2.5:7b

# 3. Установить OpenCV и GoCV (см. https://gocv.io/getting-started/)

# 4. Создать .env файл
cp .env.example .env
# Заполнить TELEGRAM_TOKEN

# 5. Запустить
make run
```

### Запуск через Docker

```bash
# 1. Создать .env файл с TELEGRAM_TOKEN
cp .env.example .env

# 2. Запустить
docker-compose up -d

# 3. Загрузить модель в Ollama (первый раз)
docker exec vision-bot-ollama ollama pull qwen2.5:7b
```

---

## 14. Требования к окружению

### Минимальные требования

| Параметр | CPU-only | С GPU |
|----------|----------|-------|
| RAM | 16 GB | 8 GB |
| VRAM | - | 8 GB (для 7B) |
| Диск | 10 GB (модель + OpenCV) | 10 GB |
| ОС | Linux (рекомендуется), macOS | Linux |

### Рекомендуемые требования

- **RAM**: 32 GB
- **GPU**: NVIDIA с 12+ GB VRAM (для Qwen2.5:14b)
- **ОС**: Ubuntu 22.04+

---

## 15. Дальнейшее развитие

1. **Кэширование результатов** — для повторных запросов с одинаковыми изображениями
2. **Метрики и мониторинг** — Prometheus + Grafana
3. **Очередь задач** — для асинхронной обработки при высокой нагрузке
4. **Тонкая настройка детектора** — адаптация параметров под конкретный тип деталей
5. **Fine-tuning модели** — обучение на специфичных данных для улучшения описаний
