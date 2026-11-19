package bot

import (
	"context"
	"log/slog"
	"sync"
	"time"

	tele "gopkg.in/telebot.v3"
	"tg-photo-bot/processor"
)

// userState holds the per-user photo queue and settings.
// All fields are protected by mu.
type userState struct {
	mu     sync.Mutex
	mode   processor.Mode
	chatID int64
	queue  []*tele.Photo
	timer  *time.Timer
}

// QueueManager is the central in-memory registry of per-user states.
// It is safe for concurrent use from multiple goroutines.
type QueueManager struct {
	mu      sync.RWMutex
	states  map[int64]*userState

	bot     *tele.Bot
	proc    *processor.Processor
	ttl     time.Duration
	maxSize int

	wg sync.WaitGroup // tracks all active processing goroutines
}

func newQueueManager(
	bot *tele.Bot,
	proc *processor.Processor,
	ttl time.Duration,
	maxSize int,
) *QueueManager {
	return &QueueManager{
		states:  make(map[int64]*userState),
		bot:     bot,
		proc:    proc,
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// setMode updates the output mode for a user (creates the state if absent).
func (qm *QueueManager) setMode(userID int64, mode processor.Mode) {
	s := qm.getOrCreate(userID)
	s.mu.Lock()
	s.mode = mode
	s.mu.Unlock()
	slog.Info("mode set", "user_id", userID, "mode", mode)
}

// addPhoto appends photo to the user's queue and resets the debounce timer.
// Returns false (and does NOT add) when the queue is already at capacity.
func (qm *QueueManager) addPhoto(userID, chatID int64, photo *tele.Photo) bool {
	s := qm.getOrCreate(userID)

	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.queue) >= qm.maxSize {
		slog.Warn("queue full, photo dropped", "user_id", userID, "max", qm.maxSize)
		return false
	}

	// Keep chatID fresh (user might message from different chats).
	s.chatID = chatID
	s.queue = append(s.queue, photo)

	// Reset debounce timer on every new photo.
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(qm.ttl, func() {
		qm.flush(userID)
	})

	slog.Debug("photo queued", "user_id", userID, "queue_len", len(s.queue))
	return true
}

// getOrCreate returns the existing state for userID or initialises a new one.
// Uses a double-checked locking pattern to minimise write-lock contention.
func (qm *QueueManager) getOrCreate(userID int64) *userState {
	qm.mu.RLock()
	s, ok := qm.states[userID]
	qm.mu.RUnlock()
	if ok {
		return s
	}

	qm.mu.Lock()
	defer qm.mu.Unlock()
	if s, ok = qm.states[userID]; ok { // re-check under write lock
		return s
	}
	s = &userState{mode: processor.ModeZip}
	qm.states[userID] = s
	return s
}

// flush atomically snapshots and clears the user's queue, then spawns a
// processing goroutine. If the queue is already empty (e.g. after a race
// between the timer callback and shutdown), it is a no-op.
func (qm *QueueManager) flush(userID int64) {
	qm.mu.RLock()
	s, ok := qm.states[userID]
	qm.mu.RUnlock()
	if !ok {
		return
	}

	s.mu.Lock()
	if len(s.queue) == 0 {
		s.mu.Unlock()
		return
	}
	// Atomic snapshot: copy queue and reset in one critical section.
	photos := make([]*tele.Photo, len(s.queue))
	copy(photos, s.queue)
	s.queue = s.queue[:0]
	chatID := s.chatID
	mode := s.mode
	s.mu.Unlock()

	slog.Info("flushing queue", "user_id", userID, "photos", len(photos), "mode", mode)

	qm.wg.Add(1)
	go func() {
		defer qm.wg.Done()
		// Use context.Background() so that in-flight downloads are NOT
		// cancelled when the shutdown signal arrives — we want to finish
		// delivering files that are already being downloaded.
		qm.proc.Process(context.Background(), qm.bot, userID, chatID, photos, mode)
	}()
}

// shutdown stops all pending timers, flushes any remaining queues, and blocks
// until every processing goroutine has returned.
func (qm *QueueManager) shutdown() {
	slog.Info("queue manager: flushing pending queues before shutdown")

	qm.mu.Lock()
	userIDs := make([]int64, 0, len(qm.states))
	for uid, s := range qm.states {
		s.mu.Lock()
		if s.timer != nil {
			s.timer.Stop() // prevent AfterFunc from firing after we flush manually
			s.timer = nil
		}
		s.mu.Unlock()
		userIDs = append(userIDs, uid)
	}
	qm.mu.Unlock()

	// Flush each user's remaining queue (no-op if already empty).
	for _, uid := range userIDs {
		qm.flush(uid)
	}

	slog.Info("queue manager: waiting for goroutines...")
	qm.wg.Wait()
}
