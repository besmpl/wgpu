//go:build windows && !(js && wasm)

package dx12

import (
	"testing"
	"unsafe"

	"github.com/besmpl/wgpu/hal/dx12/d3d12"
)

func TestTransitionTextureIfNeededEmitsCopySourceBarrier(t *testing.T) {
	texture := &Texture{
		raw:          &d3d12.ID3D12Resource{},
		currentState: d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET,
	}
	var got d3d12.D3D12_RESOURCE_BARRIER
	old := transitionTextureResourceBarrier
	transitionTextureResourceBarrier = func(_ *d3d12.ID3D12GraphicsCommandList, barrier *d3d12.D3D12_RESOURCE_BARRIER) {
		got = *barrier
	}
	defer func() { transitionTextureResourceBarrier = old }()

	(&CommandEncoder{}).transitionTextureIfNeeded(texture, d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE)
	if got.Type != d3d12.D3D12_RESOURCE_BARRIER_TYPE_TRANSITION {
		t.Fatalf("barrier type = %d, want transition", got.Type)
	}
	transition := (*d3d12.D3D12_RESOURCE_TRANSITION_BARRIER)(unsafe.Pointer(&got.Union[0]))
	if transition.StateBefore != d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET || transition.StateAfter != d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE {
		t.Fatalf("barrier states = %d -> %d, want RENDER_TARGET -> COPY_SOURCE", transition.StateBefore, transition.StateAfter)
	}
	if texture.currentState != d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE {
		t.Fatalf("tracked texture state = %d, want COPY_SOURCE", texture.currentState)
	}
}

func TestNeedsExplicitTextureBarrier(t *testing.T) {
	tests := []struct {
		name         string
		current      d3d12.D3D12_RESOURCE_STATES
		target       d3d12.D3D12_RESOURCE_STATES
		needsBarrier bool
	}{
		{
			name:         "same state",
			current:      d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE,
			target:       d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE,
			needsBarrier: false,
		},
		{
			name:         "common to render target is implicit promotion",
			current:      d3d12.D3D12_RESOURCE_STATE_COMMON,
			target:       d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET,
			needsBarrier: false,
		},
		{
			name:         "render target to copy source",
			current:      d3d12.D3D12_RESOURCE_STATE_RENDER_TARGET,
			target:       d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE,
			needsBarrier: true,
		},
		{
			name:         "common to copy source",
			current:      d3d12.D3D12_RESOURCE_STATE_COMMON,
			target:       d3d12.D3D12_RESOURCE_STATE_COPY_SOURCE,
			needsBarrier: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsExplicitTextureBarrier(tt.current, tt.target); got != tt.needsBarrier {
				t.Fatalf("needsExplicitTextureBarrier(%d, %d) = %t, want %t", tt.current, tt.target, got, tt.needsBarrier)
			}
		})
	}
}
