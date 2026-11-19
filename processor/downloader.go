package processor

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	tele "gopkg.in/telebot.v3"
)

// DownloadAll fetches every photo in parallel and returns local file paths
// in the same order as the input slice.
// tempDir must already exist; the caller owns cleanup.
func DownloadAll(ctx context.Context, bot *tele.Bot, photos []*tele.Photo, tempDir string) ([]string, error) {
	type result struct {
		path string
		err  error
	}

	results := make([]result, len(photos))
	var wg sync.WaitGroup

	// Limit concurrent downloads to avoid hitting Telegram API rate limits (HTTP 429).
	const maxConcurrentDownloads = 8
	sem := make(chan struct{}, maxConcurrentDownloads)

	for i, photo := range photos {
		wg.Add(1)
		go func(idx int, p *tele.Photo) {
			defer wg.Done()
			// Respect context cancellation (e.g. from a per-operation timeout).
			if ctx.Err() != nil {
				results[idx] = result{err: ctx.Err()}
				return
			}

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results[idx] = result{err: ctx.Err()}
				return
			}

			path, err := downloadOne(bot, p, tempDir, idx)
			results[idx] = result{path: path, err: err}
		}(i, photo)
	}

	wg.Wait()

	paths := make([]string, 0, len(photos))
	for i, r := range results {
		if r.err != nil {
			slog.Error("photo download failed", "index", i+1, "err", r.err)
			return nil, fmt.Errorf("download photo[%d]: %w", i+1, r.err)
		}
		paths = append(paths, r.path)
	}
	return paths, nil
}

// downloadOne fetches a single Telegram photo and writes it to tempDir.
// The filename encodes the index so order is preserved across concurrent calls.
func downloadOne(bot *tele.Bot, photo *tele.Photo, tempDir string, idx int) (string, error) {
	// FileByID resolves FilePath (needed by bot.Download).
	file, err := bot.FileByID(photo.FileID)
	if err != nil {
		return "", fmt.Errorf("get file info: %w", err)
	}

	// Filename: photo_001.jpg … photo_100.jpg  — no user-controlled input, no traversal risk.
	dest := filepath.Join(tempDir, fmt.Sprintf("photo_%03d.jpg", idx+1))

	if err = bot.Download(&file, dest); err != nil {
		_ = os.Remove(dest) // remove potentially partial file
		return "", fmt.Errorf("download to %s: %w", dest, err)
	}

	slog.Info("photo downloaded", "index", idx+1, "path", dest)
	return dest, nil
}
