// Package bot wires telebot, the command handlers, and the user-based debounce
// queue into a single coherent Bot type.
package bot

import (
	"log/slog"
	"time"

	tele "gopkg.in/telebot.v3"
	"tg-photo-bot/processor"
)

const (
	// debounceTTL is the quiet period after the last photo before the batch
	// is considered complete and processing begins.
	debounceTTL = 4 * time.Second

	// maxQueueSize is the hard cap on photos per user queue (DoS protection).
	maxQueueSize = 100
)

// Bot owns the telebot instance and the per-user queue manager.
type Bot struct {
	tb *tele.Bot
	qm *QueueManager
}

// New creates the Telegram bot, registers all handlers, and returns it ready
// to call Start. token must be a valid BotFather token.
func New(token string, proc *processor.Processor) (*Bot, error) {
	settings := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	tb, err := tele.NewBot(settings)
	if err != nil {
		return nil, err
	}

	qm := newQueueManager(tb, proc, debounceTTL, maxQueueSize)
	b := &Bot{tb: tb, qm: qm}
	b.registerHandlers()

	slog.Info("bot initialized", "username", tb.Me.Username)
	return b, nil
}

func (b *Bot) registerHandlers() {
	b.tb.Handle("/start", b.handleStart)
	b.tb.Handle("/help",  b.handleStart)
	b.tb.Handle("/zip",   b.handleZipCmd)
	b.tb.Handle("/pdf",   b.handlePdfCmd)
	b.tb.Handle(tele.OnPhoto, b.handlePhoto)
}

// Start begins long-polling. Blocks until Stop is called.
func (b *Bot) Start() {
	b.tb.Start()
}

// Stop halts Telegram long-polling so Start unblocks.
func (b *Bot) Stop() {
	b.tb.Stop()
}

// Shutdown flushes all pending user queues and waits for every
// in-flight processing goroutine to finish before returning.
func (b *Bot) Shutdown() {
	b.qm.shutdown() // flush pending timers + wg.Wait()
	slog.Info("all processing goroutines finished")
}
