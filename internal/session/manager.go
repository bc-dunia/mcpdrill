package session

import (
	"context"
	"fmt"
	"sync/atomic"
)

type Manager struct {
	config  *SessionConfig
	handler ModeHandler
	mode    SessionMode
	closed  atomic.Bool
}

func NewManager(config *SessionConfig) (*Manager, error) {
	if config == nil {
		return nil, ErrInvalidConfig
	}

	if config.Adapter == nil {
		return nil, &SessionError{Op: "create", Err: fmt.Errorf("adapter is required")}
	}

	if config.TransportConfig == nil {
		return nil, &SessionError{Op: "create", Err: fmt.Errorf("transport config is required")}
	}

	var handler ModeHandler

	switch config.Mode {
	case ModeReuse:
		handler = NewReuseMode(config)
	case ModePerRequest:
		handler = NewPerRequestMode(config)
	case ModePool:
		if config.PoolSize <= 0 {
			return nil, &SessionError{Op: "create", Err: fmt.Errorf("pool_size must be > 0 for pool mode")}
		}
		handler = NewPoolMode(config)
	case ModeChurn:
		handler = NewChurnMode(config)
	default:
		return nil, &SessionError{Op: "create", Err: fmt.Errorf("unknown session mode: %s", config.Mode)}
	}

	return &Manager{
		config:  config,
		handler: handler,
		mode:    config.Mode,
	}, nil
}

func (m *Manager) Start(ctx context.Context) {
	switch h := m.handler.(type) {
	case *ReuseMode:
		h.Start(ctx)
	case *PoolMode:
		h.Start(ctx)
	}
}

func (m *Manager) Acquire(ctx context.Context, vuID string) (*SessionInfo, error) {
	if m.closed.Load() {
		return nil, ErrManagerClosed
	}

	return m.handler.Acquire(ctx, vuID)
}

func (m *Manager) Release(ctx context.Context, session *SessionInfo) error {
	if m.closed.Load() {
		return nil
	}

	return m.handler.Release(ctx, session)
}

func (m *Manager) Invalidate(ctx context.Context, session *SessionInfo) error {
	return m.handler.Invalidate(ctx, session)
}

func (m *Manager) Close(ctx context.Context) error {
	if m.closed.Swap(true) {
		return nil
	}

	return m.handler.Close(ctx)
}

func (m *Manager) Metrics() *SessionMetrics {
	return m.handler.Metrics()
}

func (m *Manager) Mode() SessionMode {
	return m.mode
}

func (m *Manager) Config() *SessionConfig {
	return m.config
}
