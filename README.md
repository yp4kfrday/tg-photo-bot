# tg-photo-bot

Production-ready Telegram-бот на Go, который собирает фотографии от пользователя (как одиночные, так и пачки/медиагруппы) и формирует **ZIP-архив** или **A4 PDF-коллаж**.

---

## Возможности

- **Два режима работы:**
  - `/zip` — упаковка оригинальных фото в ZIP-архив (по умолчанию).
  - `/pdf` — верстка плотного A4 PDF-коллажа с автокропом по центру (сетка 4×5, до 20 фото на лист с авторазбивкой на страницы).
- **In-Memory Debounce Queue:** не требует нажатия "Готово". Бот ждет 4 секунды после последнего отправленного фото и автоматически запускает обработку всей пачки.
- **Защита от перегрузки (DoS protection):** лимит до 100 фото на пачку от одного пользователя.
- **Ограничение конкурентных соединений:** семафор на параллельные скачивания для предотвращения Telegram HTTP 429 (Rate Limit).
- **Надежная очистка ресурсов:** гарантированное удаление изолированных временных файлов (`os.MkdirTemp` + `defer os.RemoveAll`) даже при сбоях и panic.
- **Graceful Shutdown:** безопасное завершение при получении SIGINT / SIGTERM — поллер отключается, а все активные задачи завершают загрузку и отправку.
- **Systemd Hardening:** готов к запуску в изолированном окружении Linux (`PrivateTmp`, `ProtectSystem=strict`, `NoNewPrivileges`).

---

## Структура проекта

```
tg-photo-bot/
├── main.go                        # Точка входа, инициализация и graceful shutdown
├── bot/
│   ├── bot.go                    # Инициализация telebot, маршрутизация хэндлеров
│   ├── handlers.go               # Обработчики команд (/start, /zip, /pdf) и фото
│   └── queue.go                  # Потокобезопасная очередь с debounce-таймером
├── processor/
│   ├── processor.go              # Оркестрация: скачивание → генерация → отправка → очистка
│   ├── downloader.go             # Параллельное скачивание фото с пулом горутин
│   ├── archiver.go               # Упаковка файлов в ZIP без риска path-traversal
│   ├── pdfgen.go                 # Генерация A4 PDF-коллажа с автокропом (imaging.Fill)
│   └── pdfgen_test.go            # Модульные тесты архиватора и PDF-генератора
├── deploy/
│   ├── tg-photo-bot.service      # Systemd unit файл
│   └── deploy.sh                 # Скрипт автоматического деплоя на сервер
├── bootstrap.sh                  # Быстрый скрипт проверки и запуска тестов
├── .env.example                  # Пример конфигурации
├── .gitignore                    # Исключение секретов, бинарников и временных файлов
├── go.mod
└── go.sum
```

---

## Быстрый старт (локально)

### 1. Подготовка окружения

```bash
# Клонировать репозиторий
git clone https://github.com/<username>/tg-photo-bot.git
cd tg-photo-bot

# Загрузить зависимости
go mod download
```

### 2. Настройка токена

```bash
cp .env.example .env
# Отредактируйте .env и укажите ваш BOT_TOKEN от @BotFather
```

> ⚠️ **Важно:** файл `.env` содержит секретный токен и добавлен в `.gitignore`. Ни в коем случае не коммитьте его в репозиторий!

### 3. Запуск тестов и бота

```bash
# Запуск тестов
go test -v ./processor/...

# Запуск бота
go run .
```

---

## Сборка и деплой на VPS (Ubuntu / Debian)

### 1. Сборка бинарника под Linux

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags="-s -w" -o tg-photo-bot .
```

### 2. Копирование на сервер

```bash
scp tg-photo-bot user@your-vps:/tmp/
scp deploy/tg-photo-bot.service user@your-vps:/tmp/
scp .env user@your-vps:/tmp/.env
```

### 3. Установка на сервере

```bash
# Создать директорию и системного пользователя
sudo mkdir -p /opt/tg-photo-bot
sudo useradd --system --no-create-home --shell /usr/sbin/nologin tgbot

# Разместить файлы
sudo mv /tmp/tg-photo-bot /opt/tg-photo-bot/
sudo chmod +x /opt/tg-photo-bot/tg-photo-bot
sudo mv /tmp/.env /opt/tg-photo-bot/.env

# Права доступа
sudo chown -R tgbot:tgbot /opt/tg-photo-bot
sudo chmod 600 /opt/tg-photo-bot/.env

# Настройка systemd
sudo mv /tmp/tg-photo-bot.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now tg-photo-bot

# Проверить статус
sudo systemctl status tg-photo-bot
```

### 4. Управление и логи

```bash
# Просмотр логов в реальном времени
sudo journalctl -u tg-photo-bot -f

# Перезапуск сервиса
sudo systemctl restart tg-photo-bot
```

---

## Переменные окружения

| Переменная  | Описание                     | По умолчанию | Обязательная |
|-------------|------------------------------|--------------|--------------|
| `BOT_TOKEN` | Токен Telegram-бота от @BotFather | —       | ✅ Да        |

---

## Лицензия

MIT License.
