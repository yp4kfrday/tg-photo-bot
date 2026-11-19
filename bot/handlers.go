package bot

import (
	"tg-photo-bot/processor"

	tele "gopkg.in/telebot.v3"
)

const startText = `👋 Привет! Отправь мне фотографии — я соберу их в архив.

Режимы:
/zip — упаковать в ZIP (по умолчанию)
/pdf — сформировать A4 PDF-документ (создание коллажа)

После выбора режима просто отправляй фото. Я подожду 4 секунды после последнего - затем обработаю всю пачку.`

func (b *Bot) handleStart(c tele.Context) error {
	return c.Send(startText)
}

func (b *Bot) handleZipCmd(c tele.Context) error {
	b.qm.setMode(c.Sender().ID, processor.ModeZip)
	return c.Send("✅ Режим: ZIP-архив. Отправляй фотографии!")
}

func (b *Bot) handlePdfCmd(c tele.Context) error {
	b.qm.setMode(c.Sender().ID, processor.ModePdf)
	return c.Send("✅ Режим: PDF-документ. Отправляй фотографии!")
}

func (b *Bot) handlePhoto(c tele.Context) error {
	msg := c.Message()
	added := b.qm.addPhoto(
		c.Sender().ID,
		msg.Chat.ID,
		msg.Photo,
	)
	if !added {
		return c.Reply(
			"⚠️ Лимит очереди превышен (максимум 100 фото). " +
				"Дождись обработки текущей пачки и попробуй снова.",
		)
	}
	return nil
}
