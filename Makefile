.PHONY: run setup

run:
	@echo "🧪 Запускаю тесты..."
	go test ./...
	@echo "🐳 Запускаю Docker..."
	docker compose up --build

setup:
	@echo "🧰 Подготовка окружения..."
	@echo "📦 Обновляю зависимости Go (go mod tidy)..."
	go mod tidy
	cp .env.example .env
	@printf "➡️ Введите Telegram токен: " && read -r TOKEN && \
	sed -i.bak "s|^TELEGRAM_TOKEN=.*|TELEGRAM_TOKEN=$$TOKEN|" .env && \
	rm -f .env.bak
	@echo "✅ Готово. Можно запускать: make run"
