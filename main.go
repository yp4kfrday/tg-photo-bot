package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"tg-photo-bot/bot"
	"tg-photo-bot/processor"
)

func main() {
	// ------------------------------------------------------------------
	// Structured JSON logging — plays well with journald / log aggregators.
	// ------------------------------------------------------------------
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	// ------------------------------------------------------------------
	// Configuration: load .env, fall back to real env vars.
	// ------------------------------------------------------------------
	if err := godotenv.Load(); err != nil {
		slog.Warn("no .env file found, relying on environment variables", "err", err)
	}

	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		slog.Error("BOT_TOKEN is not set")
		os.Exit(1)
	}

	// ------------------------------------------------------------------
	// Wire up dependencies.
	// ------------------------------------------------------------------
	proc := processor.New()

	b, err := bot.New(token, proc)
	if err != nil {
		slog.Error("failed to create bot", "err", err)
		os.Exit(1)
	}

	// ------------------------------------------------------------------
	// Graceful shutdown: listen for SIGINT / SIGTERM.
	// ------------------------------------------------------------------
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		slog.Info("shutdown signal received — stopping bot polling...")
		b.Stop()
	}()

	slog.Info("bot is running — press Ctrl+C to stop")
	b.Start() // blocks until b.Stop() is called

	slog.Info("flushing pending queues and waiting for in-flight tasks...")
	b.Shutdown() // runs on main goroutine; blocks until all processing goroutines finish

	slog.Info("shutdown complete")
}
