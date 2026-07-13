// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build !(js && wasm)

package software

import (
	"testing"

	"github.com/gogpu/gputypes"
)

func TestSoftwareDoesNotAdvertiseIndexedMultiDraw(t *testing.T) {
	instance, err := (API{}).CreateInstance(nil)
	if err != nil {
		t.Fatalf("CreateInstance() error = %v", err)
	}
	adapters := instance.EnumerateAdapters(nil)
	if len(adapters) != 1 {
		t.Fatalf("EnumerateAdapters() returned %d adapters, want one", len(adapters))
	}

	const indexedMDI = gputypes.Features(gputypes.FeatureMultiDrawIndirect | gputypes.FeatureIndirectFirstInstance)
	if got := adapters[0].Features & indexedMDI; got != 0 {
		t.Fatalf("software adapter advertises indexed MDI bits %#x, want none", got)
	}
}
