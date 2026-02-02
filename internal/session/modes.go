package session

import (
	"context"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/mcp"
	"github.com/bc-dunia/mcpdrill/internal/transport"
)

func buildInitializeParams(config *SessionConfig) *transport.InitializeParams {
	version := config.ProtocolVersion
	if version == "" {
		version = mcp.DefaultProtocolVersion
	}
	return &transport.InitializeParams{
		ProtocolVersion: version,
		Capabilities:    make(map[string]interface{}),
		ClientInfo: transport.ClientInfo{
			Name:    mcp.ClientName,
			Version: mcp.ClientVersion,
		},
	}
}

func validateProtocolVersion(config *SessionConfig, outcome *transport.OperationOutcome) error {
	if config.ProtocolVersionPolicy == mcp.VersionPolicyNone {
		return nil
	}

	result, err := transport.ParseInitializeResult(outcome.Result)
	if err != nil {
		return err
	}

	requested := config.ProtocolVersion
	if requested == "" {
		requested = mcp.DefaultProtocolVersion
	}

	policy := config.ProtocolVersionPolicy
	if policy == "" {
		policy = mcp.VersionPolicyStrict
	}

	return mcp.ValidateNegotiation(requested, result.ProtocolVersion, policy)
}

type ModeHandler interface {
	Acquire(ctx context.Context, vuID string) (*SessionInfo, error)
	Release(ctx context.Context, session *SessionInfo) error
	Invalidate(ctx context.Context, session *SessionInfo) error
	Close(ctx context.Context) error
	Metrics() *SessionMetrics
}

func closeWithLog(closer io.Closer, name string) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		log.Printf("failed to close %s: %v", name, err)
	}
}

type ReuseMode struct {
	config  *SessionConfig
	evictor *Evictor

	mu          sync.RWMutex
	sessions    map[string]*SessionInfo
	sessionToVU map[string]string // session.ID -> vuID for O(1) reverse lookup
	timers      map[string]*SessionTimer
	closed      atomic.Bool

	totalCreated atomic.Int64
	totalEvicted atomic.Int64
	reconnects   atomic.Int64
}

func NewReuseMode(config *SessionConfig) *ReuseMode {
	rm := &ReuseMode{
		config:      config,
		sessions:    make(map[string]*SessionInfo),
		sessionToVU: make(map[string]string),
		timers:      make(map[string]*SessionTimer),
	}

	rm.evictor = NewEvictor(config.TTLMs, config.MaxIdleMs, func(session *SessionInfo, reason EvictionReason) {
		rm.onEviction(session, reason)
	})

	return rm
}

func (rm *ReuseMode) Start(ctx context.Context) {
	rm.evictor.Start(ctx)
}

func (rm *ReuseMode) Acquire(ctx context.Context, vuID string) (*SessionInfo, error) {
	if rm.closed.Load() {
		return nil, ErrManagerClosed
	}

	rm.mu.RLock()
	session, exists := rm.sessions[vuID]
	timer := rm.timers[vuID]
	rm.mu.RUnlock()

	if exists {
		if session.IsExpired() || session.GetState() == StateClosed || session.GetState() == StateExpired {
			rm.Invalidate(ctx, session)
			exists = false
		}
	}

	if exists {
		session.SetState(StateActive)
		session.Touch(rm.config.MaxIdleMs)
		if timer != nil {
			timer.Touch(rm.config.MaxIdleMs)
		}
		return session, nil
	}

	session, err := rm.createSession(ctx, vuID)
	if err != nil {
		return nil, err
	}

	rm.mu.Lock()
	rm.sessions[vuID] = session
	rm.sessionToVU[session.ID] = vuID
	rm.timers[vuID] = NewSessionTimer(session, rm.config.TTLMs, rm.config.MaxIdleMs, func(s *SessionInfo, reason EvictionReason) {
		rm.onEviction(s, reason)
	})
	rm.mu.Unlock()

	rm.evictor.Track(session)
	rm.totalCreated.Add(1)

	return session, nil
}

func (rm *ReuseMode) Release(ctx context.Context, session *SessionInfo) error {
	if rm.closed.Load() {
		return nil
	}

	session.SetState(StateIdle)
	return nil
}

func (rm *ReuseMode) Invalidate(ctx context.Context, session *SessionInfo) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.evictor.Untrack(session.ID)

	if vuID, ok := rm.sessionToVU[session.ID]; ok {
		delete(rm.sessions, vuID)
		delete(rm.sessionToVU, session.ID)
		if timer, ok := rm.timers[vuID]; ok {
			timer.Stop()
			delete(rm.timers, vuID)
		}
	}

	session.SetState(StateClosed)
	if session.Connection != nil {
		closeWithLog(session.Connection, "session connection")
	}

	rm.reconnects.Add(1)
	return nil
}

func (rm *ReuseMode) Close(ctx context.Context) error {
	if rm.closed.Swap(true) {
		return nil
	}

	rm.evictor.Stop()

	rm.mu.Lock()
	defer rm.mu.Unlock()

	for _, timer := range rm.timers {
		timer.Stop()
	}
	rm.timers = make(map[string]*SessionTimer)

	for _, session := range rm.sessions {
		session.SetState(StateClosed)
		if session.Connection != nil {
			closeWithLog(session.Connection, "session connection")
		}
	}
	rm.sessions = make(map[string]*SessionInfo)
	rm.sessionToVU = make(map[string]string)

	return nil
}

func (rm *ReuseMode) Metrics() *SessionMetrics {
	rm.mu.RLock()
	active := int64(0)
	idle := int64(0)
	for _, s := range rm.sessions {
		switch s.GetState() {
		case StateActive:
			active++
		case StateIdle:
			idle++
		}
	}
	rm.mu.RUnlock()

	return &SessionMetrics{
		ActiveSessions: active,
		IdleSessions:   idle,
		TotalCreated:   rm.totalCreated.Load(),
		TotalEvicted:   rm.totalEvicted.Load(),
		TTLEvictions:   rm.evictor.TTLEvictions(),
		IdleEvictions:  rm.evictor.IdleEvictions(),
		Reconnects:     rm.reconnects.Load(),
	}
}

func (rm *ReuseMode) createSession(ctx context.Context, vuID string) (*SessionInfo, error) {
	conn, err := rm.config.Adapter.Connect(ctx, rm.config.TransportConfig)
	if err != nil {
		return nil, &SessionError{Op: "connect", Err: err}
	}

	params := buildInitializeParams(rm.config)

	outcome, err := conn.Initialize(ctx, params)
	if err != nil {
		closeWithLog(conn, "connection")
		return nil, &SessionError{Op: "initialize", Err: err}
	}

	if !outcome.OK {
		closeWithLog(conn, "connection")
		if outcome.Error != nil {
			return nil, &SessionError{Op: "initialize", Err: outcome.Error}
		}
		return nil, &SessionError{Op: "initialize", Err: errSessionClosed}
	}

	if err := validateProtocolVersion(rm.config, outcome); err != nil {
		closeWithLog(conn, "connection")
		return nil, &SessionError{Op: "version_negotiation", Err: err}
	}

	_, err = conn.SendInitialized(ctx)
	if err != nil {
		closeWithLog(conn, "connection")
		return nil, &SessionError{Op: "send_initialized", Err: err}
	}

	sessionID := conn.SessionID()
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	session := NewSessionInfo(sessionID, conn, rm.config.TTLMs, rm.config.MaxIdleMs)
	session.VUID = vuID

	return session, nil
}

func (rm *ReuseMode) onEviction(session *SessionInfo, reason EvictionReason) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if vuID, ok := rm.sessionToVU[session.ID]; ok {
		delete(rm.sessions, vuID)
		delete(rm.sessionToVU, session.ID)
		if timer, ok := rm.timers[vuID]; ok {
			timer.Stop()
			delete(rm.timers, vuID)
		}
	}

	if session.Connection != nil {
		closeWithLog(session.Connection, "session connection")
	}

	rm.totalEvicted.Add(1)
}

type PerRequestMode struct {
	config *SessionConfig

	closed atomic.Bool

	totalCreated atomic.Int64
}

func NewPerRequestMode(config *SessionConfig) *PerRequestMode {
	return &PerRequestMode{
		config: config,
	}
}

func (pm *PerRequestMode) Acquire(ctx context.Context, vuID string) (*SessionInfo, error) {
	if pm.closed.Load() {
		return nil, ErrManagerClosed
	}

	session, err := pm.createSession(ctx, vuID)
	if err != nil {
		return nil, err
	}

	pm.totalCreated.Add(1)
	return session, nil
}

func (pm *PerRequestMode) Release(ctx context.Context, session *SessionInfo) error {
	session.SetState(StateClosed)
	if session.Connection != nil {
		closeWithLog(session.Connection, "session connection")
	}
	return nil
}

func (pm *PerRequestMode) Invalidate(ctx context.Context, session *SessionInfo) error {
	return pm.Release(ctx, session)
}

func (pm *PerRequestMode) Close(ctx context.Context) error {
	pm.closed.Store(true)
	return nil
}

func (pm *PerRequestMode) Metrics() *SessionMetrics {
	return &SessionMetrics{
		TotalCreated: pm.totalCreated.Load(),
	}
}

func (pm *PerRequestMode) createSession(ctx context.Context, vuID string) (*SessionInfo, error) {
	conn, err := pm.config.Adapter.Connect(ctx, pm.config.TransportConfig)
	if err != nil {
		return nil, &SessionError{Op: "connect", Err: err}
	}

	params := buildInitializeParams(pm.config)

	outcome, err := conn.Initialize(ctx, params)
	if err != nil {
		closeWithLog(conn, "connection")
		return nil, &SessionError{Op: "initialize", Err: err}
	}

	if !outcome.OK {
		closeWithLog(conn, "connection")
		if outcome.Error != nil {
			return nil, &SessionError{Op: "initialize", Err: outcome.Error}
		}
		return nil, &SessionError{Op: "initialize", Err: errSessionClosed}
	}

	if err := validateProtocolVersion(pm.config, outcome); err != nil {
		closeWithLog(conn, "connection")
		return nil, &SessionError{Op: "version_negotiation", Err: err}
	}

	_, err = conn.SendInitialized(ctx)
	if err != nil {
		closeWithLog(conn, "connection")
		return nil, &SessionError{Op: "send_initialized", Err: err}
	}

	sessionID := conn.SessionID()
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	session := NewSessionInfo(sessionID, conn, 0, 0)
	session.VUID = vuID

	return session, nil
}

type PoolMode struct {
	config *SessionConfig
	pool   *SessionPool

	closed atomic.Bool
}

func NewPoolMode(config *SessionConfig) *PoolMode {
	return &PoolMode{
		config: config,
		pool:   NewSessionPool(config.PoolSize, config.TTLMs, config.MaxIdleMs),
	}
}

func (pm *PoolMode) Start(ctx context.Context) {
	pm.pool.Start(ctx)
}

func (pm *PoolMode) Acquire(ctx context.Context, vuID string) (*SessionInfo, error) {
	if pm.closed.Load() {
		return nil, ErrManagerClosed
	}

	session, needsCreate, err := pm.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	if needsCreate {
		session, err = pm.createSession(ctx)
		if err != nil {
			return nil, err
		}
		pm.pool.Add(session)
	}

	return session, nil
}

func (pm *PoolMode) Release(ctx context.Context, session *SessionInfo) error {
	if pm.closed.Load() {
		if session.Connection != nil {
			closeWithLog(session.Connection, "session connection")
		}
		return nil
	}

	pm.pool.Release(session)
	return nil
}

func (pm *PoolMode) Invalidate(ctx context.Context, session *SessionInfo) error {
	pm.pool.Remove(session)
	return nil
}

func (pm *PoolMode) Close(ctx context.Context) error {
	if pm.closed.Swap(true) {
		return nil
	}

	pm.pool.Close()
	return nil
}

func (pm *PoolMode) Metrics() *SessionMetrics {
	return &SessionMetrics{
		ActiveSessions: int64(pm.pool.InUse()),
		IdleSessions:   int64(pm.pool.Available()),
		TotalCreated:   pm.pool.TotalCreated(),
		TTLEvictions:   pm.pool.TTLEvictions(),
		IdleEvictions:  pm.pool.IdleEvictions(),
		PoolWaits:      pm.pool.PoolWaits(),
		PoolTimeouts:   pm.pool.PoolTimeouts(),
	}
}

func (pm *PoolMode) createSession(ctx context.Context) (*SessionInfo, error) {
	conn, err := pm.config.Adapter.Connect(ctx, pm.config.TransportConfig)
	if err != nil {
		return nil, &SessionError{Op: "connect", Err: err}
	}

	params := buildInitializeParams(pm.config)

	outcome, err := conn.Initialize(ctx, params)
	if err != nil {
		closeWithLog(conn, "connection")
		return nil, &SessionError{Op: "initialize", Err: err}
	}

	if !outcome.OK {
		closeWithLog(conn, "connection")
		if outcome.Error != nil {
			return nil, &SessionError{Op: "initialize", Err: outcome.Error}
		}
		return nil, &SessionError{Op: "initialize", Err: errSessionClosed}
	}

	if err := validateProtocolVersion(pm.config, outcome); err != nil {
		closeWithLog(conn, "connection")
		return nil, &SessionError{Op: "version_negotiation", Err: err}
	}

	_, err = conn.SendInitialized(ctx)
	if err != nil {
		closeWithLog(conn, "connection")
		return nil, &SessionError{Op: "send_initialized", Err: err}
	}

	sessionID := conn.SessionID()
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	session := NewSessionInfo(sessionID, conn, pm.config.TTLMs, pm.config.MaxIdleMs)
	return session, nil
}

type ChurnMode struct {
	config           *SessionConfig
	churnInterval    time.Duration
	churnIntervalOps int64
	useOpsBased      bool

	mu          sync.RWMutex
	sessions    map[string]*churnSession
	sessionToVU map[string]string
	closed      atomic.Bool

	totalCreated atomic.Int64
	totalEvicted atomic.Int64
}

type churnSession struct {
	session   *SessionInfo
	createdAt time.Time
	opCount   int64
}

func NewChurnMode(config *SessionConfig) *ChurnMode {
	cm := &ChurnMode{
		config:      config,
		sessions:    make(map[string]*churnSession),
		sessionToVU: make(map[string]string),
	}

	if config.ChurnIntervalOps > 0 {
		cm.useOpsBased = true
		cm.churnIntervalOps = config.ChurnIntervalOps
	} else if config.ChurnIntervalMs > 0 {
		cm.churnInterval = time.Duration(config.ChurnIntervalMs) * time.Millisecond
	} else {
		cm.useOpsBased = true
		cm.churnIntervalOps = 1
	}

	return cm
}

func (cm *ChurnMode) Acquire(ctx context.Context, vuID string) (*SessionInfo, error) {
	if cm.closed.Load() {
		return nil, ErrManagerClosed
	}

	cm.mu.RLock()
	cs, exists := cm.sessions[vuID]
	cm.mu.RUnlock()

	if exists {
		shouldChurn := false
		if cm.useOpsBased {
			if cs.opCount >= cm.churnIntervalOps {
				shouldChurn = true
			}
		} else {
			if time.Since(cs.createdAt) >= cm.churnInterval {
				shouldChurn = true
			}
		}

		if shouldChurn || cs.session.IsExpired() || cs.session.GetState() == StateClosed {
			cm.Invalidate(ctx, cs.session)
			exists = false
		}
	}

	if exists {
		cs.session.SetState(StateActive)
		cs.session.Touch(cm.config.MaxIdleMs)
		return cs.session, nil
	}

	session, err := cm.createSession(ctx, vuID)
	if err != nil {
		return nil, err
	}

	cm.mu.Lock()
	cm.sessions[vuID] = &churnSession{
		session:   session,
		createdAt: time.Now(),
		opCount:   0,
	}
	cm.sessionToVU[session.ID] = vuID
	cm.mu.Unlock()

	cm.totalCreated.Add(1)
	return session, nil
}

func (cm *ChurnMode) Release(ctx context.Context, session *SessionInfo) error {
	if cm.closed.Load() {
		return nil
	}

	session.SetState(StateIdle)

	if cm.useOpsBased {
		cm.mu.Lock()
		if vuID, ok := cm.sessionToVU[session.ID]; ok {
			if cs, exists := cm.sessions[vuID]; exists {
				cs.opCount++
			}
		}
		cm.mu.Unlock()
	}

	return nil
}

func (cm *ChurnMode) Invalidate(ctx context.Context, session *SessionInfo) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if vuID, ok := cm.sessionToVU[session.ID]; ok {
		delete(cm.sessions, vuID)
		delete(cm.sessionToVU, session.ID)
	}

	session.SetState(StateClosed)
	if session.Connection != nil {
		closeWithLog(session.Connection, "session connection")
	}

	cm.totalEvicted.Add(1)
	return nil
}

func (cm *ChurnMode) Close(ctx context.Context) error {
	if cm.closed.Swap(true) {
		return nil
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, cs := range cm.sessions {
		cs.session.SetState(StateClosed)
		if cs.session.Connection != nil {
			closeWithLog(cs.session.Connection, "session connection")
		}
	}
	cm.sessions = make(map[string]*churnSession)
	cm.sessionToVU = make(map[string]string)

	return nil
}

func (cm *ChurnMode) Metrics() *SessionMetrics {
	cm.mu.RLock()
	active := int64(0)
	idle := int64(0)
	for _, cs := range cm.sessions {
		switch cs.session.GetState() {
		case StateActive:
			active++
		case StateIdle:
			idle++
		}
	}
	cm.mu.RUnlock()

	return &SessionMetrics{
		ActiveSessions: active,
		IdleSessions:   idle,
		TotalCreated:   cm.totalCreated.Load(),
		TotalEvicted:   cm.totalEvicted.Load(),
	}
}

func (cm *ChurnMode) IsOpsBased() bool {
	return cm.useOpsBased
}

func (cm *ChurnMode) ChurnIntervalOps() int64 {
	return cm.churnIntervalOps
}

func (cm *ChurnMode) createSession(ctx context.Context, vuID string) (*SessionInfo, error) {
	conn, err := cm.config.Adapter.Connect(ctx, cm.config.TransportConfig)
	if err != nil {
		return nil, &SessionError{Op: "connect", Err: err}
	}

	params := buildInitializeParams(cm.config)

	outcome, err := conn.Initialize(ctx, params)
	if err != nil {
		closeWithLog(conn, "connection")
		return nil, &SessionError{Op: "initialize", Err: err}
	}

	if !outcome.OK {
		closeWithLog(conn, "connection")
		if outcome.Error != nil {
			return nil, &SessionError{Op: "initialize", Err: outcome.Error}
		}
		return nil, &SessionError{Op: "initialize", Err: errSessionClosed}
	}

	if err := validateProtocolVersion(cm.config, outcome); err != nil {
		closeWithLog(conn, "connection")
		return nil, &SessionError{Op: "version_negotiation", Err: err}
	}

	_, err = conn.SendInitialized(ctx)
	if err != nil {
		closeWithLog(conn, "connection")
		return nil, &SessionError{Op: "send_initialized", Err: err}
	}

	sessionID := conn.SessionID()
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	session := NewSessionInfo(sessionID, conn, cm.config.TTLMs, cm.config.MaxIdleMs)
	session.VUID = vuID

	return session, nil
}

var sessionCounter atomic.Int64

func generateSessionID() string {
	n := sessionCounter.Add(1)
	return "ses_" + formatInt64(n)
}

func formatInt64(n int64) string {
	const digits = "0123456789abcdef"
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n&0xf]
		n >>= 4
	}
	if i == len(buf) {
		i--
		buf[i] = '0'
	}
	return string(buf[i:])
}
