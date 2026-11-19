#!/usr/bin/env bash
# deploy.sh — one-shot deploy script for Ubuntu VPS.
# Run as root or a user with sudo rights.
set -euo pipefail

APP_NAME="tg-photo-bot"
INSTALL_DIR="/opt/${APP_NAME}"
SERVICE_USER="tgbot"

echo "==> Building binary..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags="-s -w" -o "${APP_NAME}" .

echo "==> Preparing install directory..."
sudo mkdir -p "${INSTALL_DIR}"
sudo cp "${APP_NAME}" "${INSTALL_DIR}/"
sudo chmod +x "${INSTALL_DIR}/${APP_NAME}"

echo "==> Copying .env (skip if already present)..."
if [ ! -f "${INSTALL_DIR}/.env" ]; then
  sudo cp .env.example "${INSTALL_DIR}/.env"
  echo "    !! Edit ${INSTALL_DIR}/.env and set BOT_TOKEN before starting the service !!"
fi

echo "==> Creating system user '${SERVICE_USER}' (if not exists)..."
if ! id "${SERVICE_USER}" &>/dev/null; then
  sudo useradd --system --no-create-home --shell /usr/sbin/nologin "${SERVICE_USER}"
fi
sudo chown -R "${SERVICE_USER}:${SERVICE_USER}" "${INSTALL_DIR}"

echo "==> Installing systemd unit..."
sudo cp deploy/tg-photo-bot.service /etc/systemd/system/
sudo systemctl daemon-reload

echo "==> Enabling and starting the service..."
sudo systemctl enable --now "${APP_NAME}"
sudo systemctl status "${APP_NAME}" --no-pager

echo "==> Done. Tail logs with: sudo journalctl -u ${APP_NAME} -f"
