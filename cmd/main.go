package main

import (
	"log"

	"vision-bot/config"
	telegram "vision-bot/internal/api"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if cfg.TelegramToken == "" {
		log.Fatal("TELEGRAM_TOKEN is required")
	}

	bot, err := telegram.NewBot(cfg.TelegramToken)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	log.Println("Bot is running...")
	if err := bot.Run(); err != nil {
		log.Fatalf("Bot error: %v", err)
	}
}
