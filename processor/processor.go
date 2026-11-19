// Package processor orchestrates the download → pack → send pipeline for a
// batch of Telegram photos. It is independent of the bot framework and can be
// tested without a live Telegram connection.
package processor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	tele "gopkg.in/telebot.v3"
)

// Mode defines the output format produced for a photo batch.
type Mode int8

const (
	ModeZip Mode = iota // pack into a .zip archive (default)
	ModePdf             // render an A4 PDF document
)

func (m Mode) String() string {
	switch m {
	case ModeZip:
		return "zip"
	case ModePdf:
		return "pdf"
	default:
		return "unknown"
	}
}

// Processor orchestrates download + pack + send for a single user batch.
// It has no mutable state and is safe to call concurrently.
type Processor struct{}

// New returns a ready-to-use Processor.
func New() *Processor { return &Processor{} }

// Process is the top-level pipeline entry point:
//  1. Sends a status message to the user.
//  2. Creates a unique temporary directory (isolated per batch).
//  3. Downloads all photos in parallel.
//  4. Packages them as ZIP or PDF.
//  5. Sends the result and deletes all temporary files.
//
// The defer-based cleanup runs even on panic, guaranteeing no disk leaks.
func (p *Processor) Process(
	ctx context.Context,
	bot *tele.Bot,
	userID, chatID int64,
	photos []*tele.Photo,
	mode Mode,
) {
	log := slog.With("user_id", userID, "chat_id", chatID, "photos", len(photos), "mode", mode)
	log.Info("batch processing started")

	chat := &tele.Chat{ID: chatID}

	statusMsg, err := bot.Send(chat, fmt.Sprintf("⏳ Скачиваю %d фото...", len(photos)))
	if err != nil {
		log.Warn("could not send status message", "err", err)
	} else {
		defer func() {
			_ = bot.Delete(statusMsg)
		}()
	}

	// ── Unique temp directory ────────────────────────────────────────────────
	// Pattern: tgbot-<userID>-<nanosecond-timestamp>-XXXXXXXX
	// The userID + timestamp prefix prevents any two concurrent batches from
	// sharing the same directory even if the OS random suffix collides.
	tempDir, err := os.MkdirTemp("", fmt.Sprintf("tgbot-%d-%d-", userID, time.Now().UnixNano()))
	if err != nil {
		log.Error("failed to create temp dir", "err", err)
		sendErr(bot, chat, "❌ Внутренняя ошибка: не удалось создать временную директорию.")
		return
	}

	// ── Guaranteed cleanup ───────────────────────────────────────────────────
	defer func() {
		if r := recover(); r != nil {
			log.Error("panic recovered in processor", "panic", r)
		}
		if rerr := os.RemoveAll(tempDir); rerr != nil {
			log.Error("failed to remove temp dir", "dir", tempDir, "err", rerr)
		} else {
			log.Info("temp dir cleaned", "dir", tempDir)
		}
	}()

	// ── Download ─────────────────────────────────────────────────────────────
	log.Info("downloading photos", "dir", tempDir)
	paths, err := DownloadAll(ctx, bot, photos, tempDir)
	if err != nil {
		log.Error("download failed", "err", err)
		sendErr(bot, chat, "❌ Не удалось скачать одно или несколько фото.")
		return
	}

	// ── Package & send ───────────────────────────────────────────────────────
	switch mode {
	case ModeZip:
		err = p.sendZip(bot, chat, paths, tempDir, log)
	case ModePdf:
		err = p.sendPdf(bot, chat, paths, tempDir, log)
	}

	if err != nil {
		log.Error("packaging or sending failed", "err", err)
		sendErr(bot, chat, "❌ Не удалось подготовить или отправить файл.")
		return
	}

	log.Info("batch processing complete")
}

// ── Internal helpers ─────────────────────────────────────────────────────────

func (p *Processor) sendZip(
	bot *tele.Bot,
	chat *tele.Chat,
	paths []string,
	tempDir string,
	log *slog.Logger,
) error {
	zipPath := filepath.Join(tempDir, "album.zip")
	log.Info("creating ZIP", "path", zipPath)

	if err := Pack(paths, zipPath); err != nil {
		return fmt.Errorf("pack zip: %w", err)
	}

	doc := &tele.Document{
		File:     tele.FromDisk(zipPath),
		FileName: fmt.Sprintf("photos_%d.zip", len(paths)),
		Caption:  fmt.Sprintf("📦 %d фото", len(paths)),
	}
	_, err := bot.Send(chat, doc)
	return err
}

func (p *Processor) sendPdf(
	bot *tele.Bot,
	chat *tele.Chat,
	paths []string,
	tempDir string,
	log *slog.Logger,
) error {
	pdfPath := filepath.Join(tempDir, "album.pdf")
	log.Info("generating PDF", "path", pdfPath)

	if err := GeneratePDF(paths, tempDir, pdfPath); err != nil {
		return fmt.Errorf("generate pdf: %w", err)
	}

	doc := &tele.Document{
		File:     tele.FromDisk(pdfPath),
		FileName: fmt.Sprintf("photos_%d.pdf", len(paths)),
		Caption:  fmt.Sprintf("📄 %d фото", len(paths)),
	}
	_, err := bot.Send(chat, doc)
	return err
}

// sendErr delivers an error message to the user, logging any send failure.
func sendErr(bot *tele.Bot, chat *tele.Chat, text string) {
	if _, err := bot.Send(chat, text); err != nil {
		slog.Error("failed to deliver error message", "chat_id", chat.ID, "err", err)
	}
}
