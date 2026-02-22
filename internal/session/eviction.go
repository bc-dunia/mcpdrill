package session

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/config"
)

type EvictionReason string

const (
	EvictionTTL  EvictionReason = "ttl"
	EvictionIdle EvictionReason = "idle"
)

type EvictionCallback func(session *SessionInfo, reason EvictionReason)

type Evictor struct {
	ttlMs       int64
	maxIdleMs   int64
	checkPeriod time.Duration
	callback    EvictionCallback

	sessions sync.Map
	closed   atomic.Bool
	stopCh   chan struct{}
	wg       sync.WaitGroup

	ttlEvictions  atomic.Int64
	idleEvictions atomic.Int64
}

func NewEvictor(ttlMs, maxIdleMs int64, callback EvictionCallback) *Evictor {
	checkPeriod := time.Second
	if ttlMs > 0 && ttlMs < config.MinSessionTimeoutMs {
		checkPeriod = time.Duration(ttlMs) * time.Millisecond / 2
	}
	if maxIdleMs > 0 && maxIdleMs < config.MinSessionTimeoutMs {
		period := time.Duration(maxIdleMs) * time.Millisecond / 2
		if period < checkPeriod {
			checkPeriod = period
		}
	}

	return &Evictor{
		ttlMs:       ttlMs,
		maxIdleMs:   maxIdleMs,
		checkPeriod: checkPeriod,
		callback:    callback,
		stopCh:      make(chan struct{}),
	}
}

func (e *Evictor) Start(ctx context.Context) {
	if e.ttlMs == 0 && e.maxIdleMs == 0 {
		return
	}

	e.wg.Add(1)
	go e.evictionLoop(ctx)
}

func (e *Evictor) Stop() {
	if e.closed.Swap(true) {
		return
	}
	close(e.stopCh)
	e.wg.Wait()
}

func (e *Evictor) Track(session *SessionInfo) {
	if e.closed.Load() {
		return
	}
	e.sessions.Store(session.ID, session)
}

func (e *Evictor) Untrack(sessionID string) {
	e.sessions.Delete(sessionID)
}

func (e *Evictor) TTLEvictions() int64 {
	return e.ttlEvictions.Load()
}

func (e *Evictor) IdleEvictions() int64 {
	return e.idleEvictions.Load()
}

func (e *Evictor) evictionLoop(ctx context.Context) {
	defer e.wg.Done()

	ticker := time.NewTicker(e.checkPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.checkEvictions()
		}
	}
}

func (e *Evictor) checkEvictions() {
	now := time.Now()
	var toEvict []*evictionEntry

	e.sessions.Range(func(key, value interface{}) bool {
		session := value.(*SessionInfo)
		session.mu.RLock()
		state := session.State
		expiresAt := session.ExpiresAt
		idleExpiresAt := session.IdleExpiresAt
		session.mu.RUnlock()

		if state == StateExpired || state == StateClosed {
			e.sessions.Delete(key)
			return true
		}

		if e.ttlMs > 0 && !expiresAt.IsZero() && now.After(expiresAt) {
			toEvict = append(toEvict, &evictionEntry{session: session, reason: EvictionTTL})
			return true
		}

		if e.maxIdleMs > 0 && !idleExpiresAt.IsZero() && now.After(idleExpiresAt) {
			if state == StateIdle {
				toEvict = append(toEvict, &evictionEntry{session: session, reason: EvictionIdle})
			}
		}

		return true
	})

	for _, entry := range toEvict {
		e.evictSession(entry.session, entry.reason)
	}
}

func (e *Evictor) evictSession(session *SessionInfo, reason EvictionReason) {
	now := time.Now()

	// Re-check state and timestamps under lock to close the TOCTOU window
	// between checkEvictions snapshot and actual eviction.
	session.mu.Lock()
	state := session.State
	if state == StateExpired || state == StateClosed {
		session.mu.Unlock()
		e.sessions.Delete(session.ID)
		return
	}
	switch reason {
	case EvictionIdle:
		if state != StateIdle || session.IdleExpiresAt.IsZero() || !now.After(session.IdleExpiresAt) {
			session.mu.Unlock()
			return
		}
	case EvictionTTL:
		if state == StateActive || session.ExpiresAt.IsZero() || !now.After(session.ExpiresAt) {
			session.mu.Unlock()
			return
		}
	}
	session.State = StateExpired
	session.mu.Unlock()

	e.sessions.Delete(session.ID)

	switch reason {
	case EvictionTTL:
		e.ttlEvictions.Add(1)
	case EvictionIdle:
		e.idleEvictions.Add(1)
	}

	if e.callback != nil {
		e.callback(session, reason)
	}
}

type evictionEntry struct {
	session *SessionInfo
	reason  EvictionReason
}

type SessionTimer struct {
	session   *SessionInfo
	ttlTimer  *time.Timer
	idleTimer *time.Timer
	callback  EvictionCallback
	mu        sync.Mutex
	stopped   bool
}

func NewSessionTimer(session *SessionInfo, ttlMs, maxIdleMs int64, callback EvictionCallback) *SessionTimer {
	st := &SessionTimer{
		session:  session,
		callback: callback,
	}
	st.mu.Lock()
	defer st.mu.Unlock()

	if ttlMs > 0 {
		st.ttlTimer = time.AfterFunc(time.Duration(ttlMs)*time.Millisecond, func() {
			st.onTTLExpired()
		})
	}

	if maxIdleMs > 0 {
		st.idleTimer = time.AfterFunc(time.Duration(maxIdleMs)*time.Millisecond, func() {
			st.onIdleExpired()
		})
	}

	return st
}

func (st *SessionTimer) Touch(maxIdleMs int64) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.stopped {
		return
	}

	if st.idleTimer != nil && maxIdleMs > 0 {
		st.idleTimer.Reset(time.Duration(maxIdleMs) * time.Millisecond)
	}
}

func (st *SessionTimer) Stop() {
	st.mu.Lock()
	defer st.mu.Unlock()

	if st.stopped {
		return
	}
	st.stopped = true

	if st.ttlTimer != nil {
		st.ttlTimer.Stop()
	}
	if st.idleTimer != nil {
		st.idleTimer.Stop()
	}
}

func (st *SessionTimer) onTTLExpired() {
	st.mu.Lock()
	if st.stopped {
		st.mu.Unlock()
		return
	}

	st.stopped = true
	if st.idleTimer != nil {
		st.idleTimer.Stop()
	}
	st.mu.Unlock()

	// Atomically transition to expired under session lock.
	// If already expired/closed (evictor won the race), skip callback.
	st.session.mu.Lock()
	prevState := st.session.State
	if prevState == StateExpired || prevState == StateClosed {
		st.session.mu.Unlock()
		return
	}
	st.session.State = StateExpired
	st.session.mu.Unlock()

	if prevState != StateActive && st.callback != nil {
		st.callback(st.session, EvictionTTL)
	}
}

func (st *SessionTimer) onIdleExpired() {
	st.mu.Lock()
	if st.stopped {
		st.mu.Unlock()
		return
	}
	st.mu.Unlock()

	st.session.mu.Lock()
	if st.session.State != StateIdle {
		st.session.mu.Unlock()
		return
	}
	st.session.State = StateExpired
	st.session.mu.Unlock()

	st.mu.Lock()
	st.stopped = true
	if st.ttlTimer != nil {
		st.ttlTimer.Stop()
	}
	st.mu.Unlock()

	if st.callback != nil {
		st.callback(st.session, EvictionIdle)
	}
}
