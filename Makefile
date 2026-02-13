.PHONY: run init-env

run:
	docker compose up --build

setup:
	cp .env.example .env
	@printf "➡️ Введите Telegram токен: " && read -r TOKEN && \
	sed -i.bak "s|^TELEGRAM_TOKEN=.*|TELEGRAM_TOKEN=$$TOKEN|" .env && \
	rm -f .env.bak
