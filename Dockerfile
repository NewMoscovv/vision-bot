# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Копируем go.mod и go.sum
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Собираем бинарник
RUN CGO_ENABLED=0 GOOS=linux go build -o /bot ./cmd/main.go

# Runtime stage
FROM alpine:latest

# Устанавливаем CA-сертификаты
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Копируем бинарник
COPY --from=builder /bot /app/bot

# Запуск
CMD ["/app/bot"]
