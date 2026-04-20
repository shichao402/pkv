package bw

import (
	"strings"
	"sync"
	"time"
)

const syncReuseWindow = 5 * time.Second

var syncNow = time.Now

// Keep recent sync state at package scope because each Cobra command creates a
// fresh Client, but interactive mode still runs in one long-lived process.
var processSyncCache syncCache

type syncCache struct {
	mu       sync.Mutex
	session  string
	syncedAt time.Time
}

func shouldReuseRecentSync(session string, now time.Time) bool {
	session = strings.TrimSpace(session)
	if session == "" {
		return false
	}

	processSyncCache.mu.Lock()
	defer processSyncCache.mu.Unlock()

	if processSyncCache.session != session || processSyncCache.syncedAt.IsZero() {
		return false
	}

	return now.Sub(processSyncCache.syncedAt) < syncReuseWindow
}

func markRecentSync(session string, now time.Time) {
	processSyncCache.mu.Lock()
	defer processSyncCache.mu.Unlock()

	processSyncCache.session = strings.TrimSpace(session)
	processSyncCache.syncedAt = now
}

func invalidateSyncCache() {
	processSyncCache.mu.Lock()
	defer processSyncCache.mu.Unlock()

	processSyncCache.session = ""
	processSyncCache.syncedAt = time.Time{}
}
