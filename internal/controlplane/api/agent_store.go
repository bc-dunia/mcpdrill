package api

import (
	"context"
	"errors"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/bc-dunia/mcpdrill/internal/agent"
)

const (
	defaultMaxSamplesPerAgent = 1000
	MaxSamplesPerRequest      = 10000
)

var ErrTooManySamples = errors.New("too many samples in request")

// AgentInfo holds connected agent information
type AgentInfo struct {
	AgentID      string            `json:"agent_id"`
	PairKey      string            `json:"pair_key"`
	Tags         map[string]string `json:"tags,omitempty"`
	Hostname     string            `json:"hostname"`
	OS           string            `json:"os"`
	Arch         string            `json:"arch"`
	Version      string            `json:"version"`
	RegisteredAt time.Time         `json:"registered_at"`
	LastSeen     time.Time         `json:"last_seen"`
	Online       bool              `json:"online"`
}

// AgentMetricsSample stores a single metrics sample
type AgentMetricsSample struct {
	Timestamp int64                 `json:"timestamp"`
	Host      *agent.HostMetrics    `json:"host,omitempty"`
	Process   *agent.ProcessMetrics `json:"process,omitempty"`
}

// AgentStore manages agent state and metrics
type AgentStore struct {
	mu                   sync.RWMutex
	agents               map[string]*AgentInfo           // agent_id -> info
	agentsByPairKey      map[string][]string             // pair_key -> []agent_id
	metrics              map[string][]AgentMetricsSample // agent_id -> samples (ring buffer)
	maxSamplesPerAgent   int
	maxSamplesPerRequest int
	agentTimeout         time.Duration
}

// NewAgentStore creates a new AgentStore with default settings
func NewAgentStore() *AgentStore {
	return &AgentStore{
		agents:               make(map[string]*AgentInfo),
		agentsByPairKey:      make(map[string][]string),
		metrics:              make(map[string][]AgentMetricsSample),
		maxSamplesPerAgent:   defaultMaxSamplesPerAgent,
		maxSamplesPerRequest: MaxSamplesPerRequest,
		agentTimeout:         5 * time.Minute,
	}
}

func NewAgentStoreWithConfig(maxSamples int, timeout time.Duration) *AgentStore {
	if maxSamples <= 0 {
		maxSamples = defaultMaxSamplesPerAgent
	}
	return &AgentStore{
		agents:               make(map[string]*AgentInfo),
		agentsByPairKey:      make(map[string][]string),
		metrics:              make(map[string][]AgentMetricsSample),
		maxSamplesPerAgent:   maxSamples,
		maxSamplesPerRequest: MaxSamplesPerRequest,
		agentTimeout:         timeout,
	}
}

// StartCleanup starts a background goroutine that marks stale agents offline
func (s *AgentStore) StartCleanup(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.CleanupStale(s.agentTimeout)
			}
		}
	}()
}

// Register registers a new agent or updates an existing one
func (s *AgentStore) Register(agentID, pairKey, hostname, os, arch, version string, tags map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Check if agent already exists
	if existing, ok := s.agents[agentID]; ok {
		// Save old pair key before updating
		oldPairKey := existing.PairKey

		// Update existing agent
		existing.PairKey = pairKey
		existing.Hostname = hostname
		existing.OS = os
		existing.Arch = arch
		existing.Version = version
		existing.Tags = tags
		existing.LastSeen = now
		existing.Online = true

		// Update pair key index if changed
		s.updatePairKeyIndex(agentID, oldPairKey, pairKey)
		return nil
	}

	// Create new agent
	info := &AgentInfo{
		AgentID:      agentID,
		PairKey:      pairKey,
		Tags:         tags,
		Hostname:     hostname,
		OS:           os,
		Arch:         arch,
		Version:      version,
		RegisteredAt: now,
		LastSeen:     now,
		Online:       true,
	}

	s.agents[agentID] = info

	// Add to pair key index
	if pairKey != "" {
		s.agentsByPairKey[pairKey] = append(s.agentsByPairKey[pairKey], agentID)
	}

	return nil
}

// updatePairKeyIndex updates the pair key index when an agent's pair key changes
func (s *AgentStore) updatePairKeyIndex(agentID, oldPairKey, newPairKey string) {
	if oldPairKey == newPairKey {
		return
	}

	// Remove from old pair key
	if oldPairKey != "" {
		agents := s.agentsByPairKey[oldPairKey]
		for i, id := range agents {
			if id == agentID {
				s.agentsByPairKey[oldPairKey] = append(agents[:i], agents[i+1:]...)
				break
			}
		}
		if len(s.agentsByPairKey[oldPairKey]) == 0 {
			delete(s.agentsByPairKey, oldPairKey)
		}
	}

	// Add to new pair key
	if newPairKey != "" {
		s.agentsByPairKey[newPairKey] = append(s.agentsByPairKey[newPairKey], agentID)
	}
}

// UpdateLastSeen updates the last seen time for an agent
func (s *AgentStore) UpdateLastSeen(agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if info, ok := s.agents[agentID]; ok {
		info.LastSeen = time.Now()
		info.Online = true
	}
}

// IngestMetrics ingests metrics samples for an agent
func (s *AgentStore) IngestMetrics(agentID string, samples []AgentMetricsSample) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	maxSamplesPerRequest := s.maxSamplesPerRequest
	if maxSamplesPerRequest <= 0 {
		maxSamplesPerRequest = MaxSamplesPerRequest
	}
	if len(samples) > maxSamplesPerRequest {
		log.Printf("[AgentStore] Rejecting metrics batch: agent=%s samples=%d limit=%d", agentID, len(samples), maxSamplesPerRequest)
		return ErrTooManySamples
	}

	// Update last seen
	if info, ok := s.agents[agentID]; ok {
		info.LastSeen = time.Now()
		info.Online = true
	}

	// Get or create metrics slice
	existing := s.metrics[agentID]

	// Append new samples
	existing = append(existing, samples...)

	// Trim to max size (ring buffer behavior)
	if len(existing) > s.maxSamplesPerAgent {
		existing = existing[len(existing)-s.maxSamplesPerAgent:]
	}

	s.metrics[agentID] = existing
	return nil
}

// GetAgent returns agent info by ID
func (s *AgentStore) GetAgent(agentID string) (*AgentInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info, ok := s.agents[agentID]
	if !ok {
		return nil, false
	}

	// Return a copy to prevent mutation
	copy := *info
	if info.Tags != nil {
		copy.Tags = make(map[string]string, len(info.Tags))
		for k, v := range info.Tags {
			copy.Tags[k] = v
		}
	}
	return &copy, true
}

// ListAgents returns all registered agents
func (s *AgentStore) ListAgents() []*AgentInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*AgentInfo, 0, len(s.agents))
	for _, info := range s.agents {
		copy := *info
		if info.Tags != nil {
			copy.Tags = make(map[string]string, len(info.Tags))
			for k, v := range info.Tags {
				copy.Tags[k] = v
			}
		}
		result = append(result, &copy)
	}
	return result
}

// GetAgentsByPairKey returns all agents for a given pair key
func (s *AgentStore) GetAgentsByPairKey(pairKey string) []*AgentInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agentIDs, ok := s.agentsByPairKey[pairKey]
	if !ok {
		return nil
	}

	result := make([]*AgentInfo, 0, len(agentIDs))
	for _, agentID := range agentIDs {
		if info, ok := s.agents[agentID]; ok {
			copy := *info
			if info.Tags != nil {
				copy.Tags = make(map[string]string, len(info.Tags))
				for k, v := range info.Tags {
					copy.Tags[k] = v
				}
			}
			result = append(result, &copy)
		}
	}
	return result
}

// GetMetrics returns metrics for an agent within a time range
func (s *AgentStore) GetMetrics(agentID string, from, to int64) []AgentMetricsSample {
	s.mu.RLock()
	defer s.mu.RUnlock()

	samples, ok := s.metrics[agentID]
	if !ok {
		return nil
	}

	// Filter by time range
	var result []AgentMetricsSample
	for _, sample := range samples {
		if (from == 0 || sample.Timestamp >= from) && (to == 0 || sample.Timestamp <= to) {
			result = append(result, sample)
		}
	}
	// Sort by timestamp
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp < result[j].Timestamp
	})
	return result
}

// GetMetricsByPairKey returns metrics for all agents matching a pair key
func (s *AgentStore) GetMetricsByPairKey(pairKey string, from, to int64) []AgentMetricsSample {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agentIDs, ok := s.agentsByPairKey[pairKey]
	if !ok {
		return nil
	}

	var result []AgentMetricsSample
	for _, agentID := range agentIDs {
		samples, ok := s.metrics[agentID]
		if !ok {
			continue
		}

		for _, sample := range samples {
			if (from == 0 || sample.Timestamp >= from) && (to == 0 || sample.Timestamp <= to) {
				result = append(result, sample)
			}
		}
	}
	// Sort by timestamp
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp < result[j].Timestamp
	})
	return result
}

// MarkOffline marks an agent as offline
func (s *AgentStore) MarkOffline(agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if info, ok := s.agents[agentID]; ok {
		info.Online = false
	}
}

// CleanupStale removes agents that haven't been seen for longer than the timeout
// Returns the list of removed agent IDs
func (s *AgentStore) CleanupStale(timeout time.Duration) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if timeout == 0 {
		timeout = s.agentTimeout
	}

	cutoff := time.Now().Add(-timeout)
	var removed []string

	for agentID, info := range s.agents {
		if info.LastSeen.Before(cutoff) {
			removed = append(removed, agentID)

			// Remove from pair key index
			if info.PairKey != "" {
				agents := s.agentsByPairKey[info.PairKey]
				for i, id := range agents {
					if id == agentID {
						s.agentsByPairKey[info.PairKey] = append(agents[:i], agents[i+1:]...)
						break
					}
				}
				if len(s.agentsByPairKey[info.PairKey]) == 0 {
					delete(s.agentsByPairKey, info.PairKey)
				}
			}

			// Remove agent
			delete(s.agents, agentID)

			// Remove metrics
			delete(s.metrics, agentID)
		}
	}

	return removed
}

// SetMaxSamplesPerAgent sets the maximum number of samples to retain per agent
func (s *AgentStore) SetMaxSamplesPerAgent(max int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxSamplesPerAgent = max
}

// SetMaxSamplesPerRequest sets the maximum number of samples accepted per request.
func (s *AgentStore) SetMaxSamplesPerRequest(max int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxSamplesPerRequest = max
}

// MaxSamplesPerRequest returns the current max samples per request.
func (s *AgentStore) MaxSamplesPerRequest() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.maxSamplesPerRequest <= 0 {
		return MaxSamplesPerRequest
	}
	return s.maxSamplesPerRequest
}

// SetAgentTimeout sets the timeout after which agents are considered stale
func (s *AgentStore) SetAgentTimeout(timeout time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentTimeout = timeout
}

// AgentCount returns the number of registered agents
func (s *AgentStore) AgentCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.agents)
}

// OnlineAgentCount returns the number of online agents
func (s *AgentStore) OnlineAgentCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, info := range s.agents {
		if info.Online {
			count++
		}
	}
	return count
}
