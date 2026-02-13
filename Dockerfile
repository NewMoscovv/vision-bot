FROM gocv/opencv:4.13.0

WORKDIR /app

# Копируем go.mod и go.sum
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем бинарник (CGO включен, сборка с gocv)
RUN CGO_ENABLED=1 GOOS=linux go build -tags gocv -o /app/bot ./cmd/main.go

# Запуск
CMD ["/app/bot"]
