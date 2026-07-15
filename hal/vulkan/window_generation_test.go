// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import "testing"

func TestWindowGenerationLifecycle(t *testing.T) {
	var state windowGenerationState
	if state.canCreate(0) || state.commit(0) || state.isCurrent(0) {
		t.Fatal("zero must never be a valid Android window generation")
	}

	if !state.canCreate(1) || !state.commit(1) || !state.isCurrent(1) {
		t.Fatal("first generation did not become current")
	}
	if state.canCreate(1) || state.commit(1) {
		t.Fatal("duplicate generation was accepted")
	}

	if !state.canCreate(2) {
		t.Fatal("newer generation was rejected before creation")
	}
	if !state.isCurrent(1) {
		t.Fatal("current surface became stale before replacement committed")
	}
	if !state.commit(2) || !state.isCurrent(2) || state.isCurrent(1) {
		t.Fatal("replacement did not atomically stale the old surface")
	}

	if !state.canCreate(3) || !state.canCreate(4) {
		t.Fatal("concurrent newer generations should be eligible to create")
	}
	if !state.commit(4) || state.commit(3) || !state.isCurrent(4) {
		t.Fatal("out-of-order completion regressed the current generation")
	}
}
