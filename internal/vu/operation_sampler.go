package vu

import (
	"math/rand"
	"sync"
)

type OperationSampler struct {
	mix         *OperationMix
	totalWeight int
	rng         *rand.Rand
	mu          sync.Mutex
}

func NewOperationSampler(mix *OperationMix, seed int64) (*OperationSampler, error) {
	if mix == nil || len(mix.Operations) == 0 {
		return nil, ErrNoOperations
	}

	totalWeight := mix.TotalWeight()
	if totalWeight <= 0 {
		return nil, ErrNoOperations
	}

	return &OperationSampler{
		mix:         mix,
		totalWeight: totalWeight,
		rng:         rand.New(rand.NewSource(seed)),
	}, nil
}

func (s *OperationSampler) Sample() *OperationWeight {
	s.mu.Lock()
	defer s.mu.Unlock()

	r := s.rng.Intn(s.totalWeight)
	cumulative := 0

	for i := range s.mix.Operations {
		cumulative += s.mix.Operations[i].Weight
		if r < cumulative {
			return &s.mix.Operations[i]
		}
	}

	return &s.mix.Operations[len(s.mix.Operations)-1]
}

func (s *OperationSampler) TotalWeight() int {
	return s.totalWeight
}

func (s *OperationSampler) OperationCount() int {
	return len(s.mix.Operations)
}

type ThinkTimeSampler struct {
	config ThinkTimeConfig
	rng    *rand.Rand
	mu     sync.Mutex
}

func NewThinkTimeSampler(config ThinkTimeConfig, seed int64) *ThinkTimeSampler {
	return &ThinkTimeSampler{
		config: config,
		rng:    rand.New(rand.NewSource(seed)),
	}
}

func (s *ThinkTimeSampler) Sample() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	thinkTime := s.config.BaseMs
	if s.config.JitterMs > 0 {
		thinkTime += s.rng.Int63n(s.config.JitterMs)
	}
	return thinkTime
}

func (s *ThinkTimeSampler) BaseMs() int64 {
	return s.config.BaseMs
}

func (s *ThinkTimeSampler) JitterMs() int64 {
	return s.config.JitterMs
}
