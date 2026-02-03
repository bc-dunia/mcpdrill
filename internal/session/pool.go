package session

import (
	"container/list"
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type SessionPool struct {
	maxSize   int
	ttlMs     int64
	maxIdleMs int64

	mu             sync.Mutex
	sessions       *list.List
	inUse          map[string]*SessionInfo
	pendingCreates int
	cond           *sync.Cond
	closed         atomic.Bool

	totalCreated atomic.Int64
	poolWaits    atomic.Int64
	poolTimeouts atomic.Int64

	evictor *Evictor
}

func NewSessionPool(maxSize int, ttlMs, maxIdleMs int64) *SessionPool {
	p := &SessionPool{
		maxSize:   maxSize,
		ttlMs:     ttlMs,
		maxIdleMs: maxIdleMs,
		sessions:  list.New(),
		inUse:     make(map[string]*SessionInfo),
	}
	p.cond = sync.NewCond(&p.mu)

	p.evictor = NewEvictor(ttlMs, maxIdleMs, func(session *SessionInfo, reason EvictionReason) {
		p.removeSession(session)
	})

	return p
}

func (p *SessionPool) Start(ctx context.Context) {
	p.evictor.Start(ctx)
}

func (p *SessionPool) Close() {
	if p.closed.Swap(true) {
		return
	}

	p.evictor.Stop()

	p.mu.Lock()
	defer p.mu.Unlock()

	for e := p.sessions.Front(); e != nil; e = e.Next() {
		session := e.Value.(*SessionInfo)
		session.SetState(StateClosed)
		if session.Connection != nil {
			closeWithLog(session.Connection, "session connection")
		}
	}
	p.sessions.Init()

	for _, session := range p.inUse {
		session.SetState(StateClosed)
		if session.Connection != nil {
			closeWithLog(session.Connection, "session connection")
		}
	}
	p.inUse = make(map[string]*SessionInfo)

	p.cond.Broadcast()
}

func (p *SessionPool) Acquire(ctx context.Context) (*SessionInfo, bool, error) {
	if p.closed.Load() {
		return nil, false, ErrManagerClosed
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for {
		if p.closed.Load() {
			return nil, false, ErrManagerClosed
		}

		for e := p.sessions.Front(); e != nil; {
			session := e.Value.(*SessionInfo)
			next := e.Next()

			if session.IsExpired() || session.GetState() == StateClosed {
				p.sessions.Remove(e)
				p.evictor.Untrack(session.ID)
				if session.Connection != nil {
					closeWithLog(session.Connection, "session connection")
				}
				e = next
				continue
			}

			p.sessions.Remove(e)
			session.SetState(StateActive)
			session.Touch(p.maxIdleMs)
			p.inUse[session.ID] = session
			return session, false, nil
		}

		if p.sessions.Len()+len(p.inUse)+p.pendingCreates < p.maxSize {
			p.pendingCreates++
			return nil, true, nil
		}

		p.poolWaits.Add(1)

		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-ctx.Done():
				p.mu.Lock()
				p.cond.Broadcast()
				p.mu.Unlock()
			case <-done:
			}
		}()

		for {
			p.cond.Wait()

			if ctx.Err() != nil {
				p.poolTimeouts.Add(1)
				return nil, false, &SessionError{Op: "acquire", Err: ctx.Err()}
			}

			for e := p.sessions.Front(); e != nil; {
				session := e.Value.(*SessionInfo)
				next := e.Next()

				if session.IsExpired() || session.GetState() == StateClosed {
					p.sessions.Remove(e)
					p.evictor.Untrack(session.ID)
					if session.Connection != nil {
						closeWithLog(session.Connection, "session connection")
					}
					e = next
					continue
				}

				p.sessions.Remove(e)
				session.SetState(StateActive)
				session.Touch(p.maxIdleMs)
				p.inUse[session.ID] = session
				return session, false, nil
			}

			if p.sessions.Len()+len(p.inUse)+p.pendingCreates < p.maxSize {
				p.pendingCreates++
				return nil, true, nil
			}
		}
	}
}

func (p *SessionPool) Add(session *SessionInfo) {
	if p.closed.Load() {
		if session.Connection != nil {
			closeWithLog(session.Connection, "session connection")
		}
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.pendingCreates > 0 {
		p.pendingCreates--
	}
	p.inUse[session.ID] = session
	p.totalCreated.Add(1)
	p.evictor.Track(session)
}

func (p *SessionPool) CancelReservation() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.pendingCreates > 0 {
		p.pendingCreates--
	}
	p.cond.Signal()
}

func (p *SessionPool) Release(session *SessionInfo) {
	if p.closed.Load() {
		if session.Connection != nil {
			closeWithLog(session.Connection, "session connection")
		}
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.inUse, session.ID)

	if session.IsExpired() || session.GetState() == StateClosed {
		p.evictor.Untrack(session.ID)
		if session.Connection != nil {
			closeWithLog(session.Connection, "session connection")
		}
		p.cond.Signal()
		return
	}

	session.SetState(StateIdle)
	session.Touch(p.maxIdleMs)
	p.sessions.PushBack(session)
	p.cond.Signal()
}

func (p *SessionPool) Remove(session *SessionInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.inUse, session.ID)
	p.evictor.Untrack(session.ID)

	for e := p.sessions.Front(); e != nil; e = e.Next() {
		if e.Value.(*SessionInfo).ID == session.ID {
			p.sessions.Remove(e)
			break
		}
	}

	if session.Connection != nil {
		closeWithLog(session.Connection, "session connection")
	}

	p.cond.Signal()
}

func (p *SessionPool) removeSession(session *SessionInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.inUse, session.ID)

	for e := p.sessions.Front(); e != nil; e = e.Next() {
		if e.Value.(*SessionInfo).ID == session.ID {
			p.sessions.Remove(e)
			break
		}
	}

	if session.Connection != nil {
		closeWithLog(session.Connection, "session connection")
	}

	p.cond.Signal()
}

func (p *SessionPool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sessions.Len() + len(p.inUse)
}

func (p *SessionPool) Available() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sessions.Len()
}

func (p *SessionPool) InUse() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.inUse)
}

func (p *SessionPool) TotalCreated() int64 {
	return p.totalCreated.Load()
}

func (p *SessionPool) PoolWaits() int64 {
	return p.poolWaits.Load()
}

func (p *SessionPool) PoolTimeouts() int64 {
	return p.poolTimeouts.Load()
}

func (p *SessionPool) TTLEvictions() int64 {
	return p.evictor.TTLEvictions()
}

func (p *SessionPool) IdleEvictions() int64 {
	return p.evictor.IdleEvictions()
}

type PoolAcquireOptions struct {
	Timeout time.Duration
}

func (p *SessionPool) AcquireWithTimeout(ctx context.Context, timeout time.Duration) (*SessionInfo, bool, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return p.Acquire(ctx)
}
