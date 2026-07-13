// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !(js && wasm)

package metal

import (
	"testing"

	"github.com/gogpu/gputypes"
)

func TestMetalDoesNotAdvertiseIndexedMultiDraw(t *testing.T) {
	const indexedMDI = gputypes.Features(gputypes.FeatureMultiDrawIndirect | gputypes.FeatureIndirectFirstInstance)
	for _, supportsMetal3 := range []bool{false, true} {
		features := metalAdapterFeatures(supportsMetal3)
		if got := features & indexedMDI; got != 0 {
			t.Fatalf("metal adapter (metal3=%t) advertises indexed MDI bits %#x, want none", supportsMetal3, got)
		}
	}
}
