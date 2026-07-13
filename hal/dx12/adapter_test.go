// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import (
	"testing"

	"github.com/besmpl/wgpu/hal/dx12/d3d12"
	"github.com/gogpu/gputypes"
)

func TestSupportsIndexedExecuteIndirect(t *testing.T) {
	tests := []struct {
		name  string
		level d3d12.D3D_FEATURE_LEVEL
		want  bool
	}{
		{name: "uninitialized", level: 0, want: false},
		{name: "feature level 10.1", level: d3d12.D3D_FEATURE_LEVEL_10_1, want: false},
		{name: "legacy 11.0", level: d3d12.D3D_FEATURE_LEVEL_11_0, want: true},
		{name: "legacy 11.1", level: d3d12.D3D_FEATURE_LEVEL_11_1, want: true},
		{name: "modern 12.0", level: d3d12.D3D_FEATURE_LEVEL_12_0, want: true},
		{name: "modern 12.2", level: d3d12.D3D_FEATURE_LEVEL_12_2, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := supportsIndexedExecuteIndirect(tt.level); got != tt.want {
				t.Fatalf("supportsIndexedExecuteIndirect(%#x) = %t, want %t", tt.level, got, tt.want)
			}
		})
	}
}

func TestAdapterFeaturesAdvertiseIndexedMultiDrawOnlyWhenSupported(t *testing.T) {
	const indexedMDI = gputypes.Features(gputypes.FeatureMultiDrawIndirect | gputypes.FeatureIndirectFirstInstance)

	tests := []struct {
		name  string
		level d3d12.D3D_FEATURE_LEVEL
		want  bool
	}{
		{name: "uninitialized", level: 0, want: false},
		{name: "below D3D12", level: d3d12.D3D_FEATURE_LEVEL_10_1, want: false},
		{name: "legacy DX12", level: d3d12.D3D_FEATURE_LEVEL_11_0, want: true},
		{name: "modern DX12", level: d3d12.D3D_FEATURE_LEVEL_12_0, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &Adapter{capabilities: AdapterCapabilities{FeatureLevel: tt.level}}
			got := adapter.Features() & indexedMDI
			if tt.want && got != indexedMDI {
				t.Fatalf("Features() indexed MDI bits = %#x, want %#x", got, indexedMDI)
			}
			if !tt.want && got != 0 {
				t.Fatalf("Features() indexed MDI bits = %#x, want no MDI bits", got)
			}
		})
	}
}
