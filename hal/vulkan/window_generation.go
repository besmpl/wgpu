// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import "sync"

// windowGenerationState makes Android's host-owned window lifecycle explicit.
// Creation may race, but only a strictly newer generation can become current;
// once committed, every older surface is stale.
type windowGenerationState struct {
	mu      sync.RWMutex
	current uint64
}

func (s *windowGenerationState) canCreate(generation uint64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return generation != 0 && generation > s.current
}

func (s *windowGenerationState) commit(generation uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if generation == 0 || generation <= s.current {
		return false
	}
	s.current = generation
	return true
}

func (s *windowGenerationState) isCurrent(generation uint64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return generation != 0 && generation == s.current
}
