package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramToken string
}

func Load() (*Config, error) {
	// Загружаем .env файл (игнорируем ошибку если файла нет)
	_ = godotenv.Load()

	cfg := &Config{
		TelegramToken: os.Getenv("TELEGRAM_TOKEN"),
	}

	return cfg, nil
}
