package auth

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// devWarnInterval throttles SkipVerify warnings so a busy server doesn't
// drown its log; one loud line per minute is enough to notice.
const devWarnInterval = time.Minute

var (
	devWarnLast    atomic.Int64 // unix-nano of last warning
	devWarnLogger  *slog.Logger
	devWarnLoggerM sync.RWMutex
)

// SetDevLogger registers the slog logger used for SkipVerify warnings.
// If nil, warnings fall back to slog.Default().
func SetDevLogger(l *slog.Logger) {
	devWarnLoggerM.Lock()
	devWarnLogger = l
	devWarnLoggerM.Unlock()
}

func (v *Verifier) warnSkipVerify() {
	now := time.Now().UnixNano()
	last := devWarnLast.Load()
	if last != 0 && time.Duration(now-last) < devWarnInterval {
		return
	}
	if !devWarnLast.CompareAndSwap(last, now) {
		return // someone else won the race
	}
	devWarnLoggerM.RLock()
	l := devWarnLogger
	devWarnLoggerM.RUnlock()
	if l == nil {
		l = slog.Default()
	}
	l.Warn("AUTH SKIP VERIFY ENABLED — DEV ONLY, all tokens accepted as nova-admin")
}

func syntheticDevIdentity() *Identity {
	return &Identity{
		Subject:       "dev-skip-verify",
		PreferredName: "dev",
		Email:         "dev@localhost",
		Realm:         "dev",
		Roles:         []string{"nova-admin"},
		Scopes:        []string{"openid"},
		ExpiresAt:     time.Now().Add(time.Hour),
	}
}
