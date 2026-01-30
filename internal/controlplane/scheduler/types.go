// Package scheduler provides worker registry and capacity management for the Control Plane.
package scheduler

import (
	"time"

	"github.com/bc-dunia/mcpdrill/internal/types"
)

// WorkerID is a unique identifier for a worker.
type WorkerID string

// WorkerInfo contains all information about a registered worker.
type WorkerInfo struct {
	WorkerID          WorkerID             `json:"worker_id"`
	HostInfo          types.HostInfo       `json:"host_info"`
	Capacity          types.WorkerCapacity `json:"capacity"`
	EffectiveCapacity types.WorkerCapacity `json:"effective_capacity"` // Adjusted based on health
	Saturated         bool                 `json:"saturated"`          // True if worker is overloaded
	RegisteredAt      int64                `json:"registered_at"`
	LastHeartbeat     int64                `json:"last_heartbeat"`
	Health            *types.WorkerHealth  `json:"health,omitempty"`
}

// Copy returns a deep copy of WorkerInfo.
func (w *WorkerInfo) Copy() *WorkerInfo {
	if w == nil {
		return nil
	}
	copy := &WorkerInfo{
		WorkerID:          w.WorkerID,
		HostInfo:          w.HostInfo,
		Capacity:          w.Capacity,
		EffectiveCapacity: w.EffectiveCapacity,
		Saturated:         w.Saturated,
		RegisteredAt:      w.RegisteredAt,
		LastHeartbeat:     w.LastHeartbeat,
	}
	if w.Health != nil {
		healthCopy := *w.Health
		copy.Health = &healthCopy
	}
	return copy
}

// NowMs returns the current time in milliseconds.
func NowMs() int64 {
	return time.Now().UnixMilli()
}

// LeaseID is a unique identifier for a lease.
type LeaseID string

// LeaseState represents the current state of a lease.
type LeaseState string

const (
	LeaseStateActive  LeaseState = "active"
	LeaseStateRevoked LeaseState = "revoked"
	LeaseStateExpired LeaseState = "expired"
)

// VUIDRange represents a range of virtual user IDs [Start, End).
type VUIDRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Overlaps returns true if this range overlaps with another range.
// Empty ranges (where Start >= End) never overlap with anything.
func (r VUIDRange) Overlaps(other VUIDRange) bool {
	if r.Start >= r.End || other.Start >= other.End {
		return false
	}
	// Ranges [a, b) and [c, d) overlap if a < d && c < b
	return r.Start < other.End && other.Start < r.End
}

// Assignment represents a work assignment for a worker.
type Assignment struct {
	RunID     string    `json:"run_id"`
	StageID   string    `json:"stage_id"`
	VUIDRange VUIDRange `json:"vu_id_range"`
}

// Lease represents an assignment lease issued to a worker.
type Lease struct {
	LeaseID    LeaseID    `json:"lease_id"`
	WorkerID   WorkerID   `json:"worker_id"`
	Assignment Assignment `json:"assignment"`
	State      LeaseState `json:"state"`
	IssuedAt   int64      `json:"issued_at"`
	ExpiresAt  int64      `json:"expires_at"`
	RevokedAt  *int64     `json:"revoked_at,omitempty"`
}

// Copy returns a deep copy of Lease.
func (l *Lease) Copy() *Lease {
	if l == nil {
		return nil
	}
	copy := &Lease{
		LeaseID:    l.LeaseID,
		WorkerID:   l.WorkerID,
		Assignment: l.Assignment,
		State:      l.State,
		IssuedAt:   l.IssuedAt,
		ExpiresAt:  l.ExpiresAt,
	}
	if l.RevokedAt != nil {
		revokedAt := *l.RevokedAt
		copy.RevokedAt = &revokedAt
	}
	return copy
}
