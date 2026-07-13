//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"fmt"

	"github.com/besmpl/wgpu/core"
	"github.com/besmpl/wgpu/hal"
	"github.com/gogpu/gputypes"
)

type preparedIndexedState struct {
	core *core.PreparedIndexedCommands
	desc PreparedIndexedCommandsDescriptor
}

// PreparedIndexedCapabilities reports the complete native route available on
// this created device. It is deliberately queried from the HAL device rather
// than inferred from an adapter feature bit alone.
func (d *Device) PreparedIndexedCapabilities() PreparedIndexedCapabilities {
	if d == nil || d.released.Load() || d.core == nil {
		return PreparedIndexedCapabilities{}
	}
	halDevice := d.halDevice()
	provider, ok := halDevice.(hal.PreparedIndexedCapabilityProvider)
	if !ok {
		return PreparedIndexedCapabilities{}
	}
	if gate, ok := halDevice.(hal.PreparedIndexedFeatureGate); ok && gate.PreparedIndexedRequiresFeature() && !d.Features().Contains(gputypes.FeatureMultiDrawIndirect) {
		return PreparedIndexedCapabilities{}
	}
	caps := provider.PreparedIndexedCapabilities()
	if !caps.PreparedIndexed || caps.MaxCommands == 0 {
		return PreparedIndexedCapabilities{}
	}
	return PreparedIndexedCapabilities{PreparedIndexed: true, MaxCommands: caps.MaxCommands}
}

// PrepareIndexedCommands validates and prepares a homogeneous indexed range
// before any render pass is opened.
func (e *CommandEncoder) PrepareIndexedCommands(desc PreparedIndexedCommandsDescriptor) (*PreparedIndexedCommands, error) {
	if e == nil || e.released {
		return nil, ErrReleased
	}
	if desc.Arguments == nil || desc.IndexBuffer == nil || desc.Arguments.core == nil || desc.IndexBuffer.core == nil ||
		(desc.Arguments.released != nil && desc.Arguments.released.Load()) ||
		(desc.IndexBuffer.released != nil && desc.IndexBuffer.released.Load()) {
		return nil, ErrPreparedIndexedInvalid
	}
	if desc.Arguments.device != e.device || desc.IndexBuffer.device != e.device {
		return nil, ErrPreparedIndexedOwnership
	}
	if err := validatePreparedIndexedDescriptor(desc, 0); err != nil {
		return nil, err
	}
	caps := e.device.PreparedIndexedCapabilities()
	if !caps.PreparedIndexed {
		return nil, ErrPreparedIndexedUnsupported
	}
	if desc.Count > caps.MaxCommands {
		return nil, ErrPreparedIndexedLimit
	}
	prepared, err := e.core.PrepareIndexedCommands(desc.Arguments.coreBuffer(), desc.ArgumentsOffset, desc.Count, desc.IndexBuffer.coreBuffer(), desc.IndexOffset, desc.IndexFormat)
	if err != nil {
		return nil, mapPreparedIndexedError(err)
	}
	e.trackRef(desc.Arguments.core.Ref)
	e.trackRef(desc.IndexBuffer.core.Ref)
	e.trackBuffer(desc.Arguments)
	e.trackBuffer(desc.IndexBuffer)
	return &PreparedIndexedCommands{state: &preparedIndexedState{core: prepared, desc: desc}}, nil
}

func mapPreparedIndexedError(err error) error {
	switch {
	case errors.Is(err, hal.ErrPreparedIndexedUnsupported):
		return ErrPreparedIndexedUnsupported
	case errors.Is(err, hal.ErrPreparedIndexedLimit):
		return ErrPreparedIndexedLimit
	case errors.Is(err, hal.ErrPreparedIndexedInvalid):
		return ErrPreparedIndexedInvalid
	case errors.Is(err, hal.ErrPreparedIndexedOwnership):
		return ErrPreparedIndexedOwnership
	case errors.Is(err, hal.ErrPreparedIndexedState):
		return ErrPreparedIndexedState
	case errors.Is(err, hal.ErrPreparedIndexedBackend):
		return fmt.Errorf("%w: %v", ErrPreparedIndexedBackend, err)
	default:
		return fmt.Errorf("%w: %v", ErrPreparedIndexedBackend, err)
	}
}

// ExecutePreparedIndexedCommands consumes a prepared token exactly once in
// the render pass created by the same command encoder.
func (p *RenderPassEncoder) ExecutePreparedIndexedCommands(prepared *PreparedIndexedCommands) error {
	if p == nil || p.encoder == nil || p.encoder.released {
		return ErrReleased
	}
	if prepared == nil || prepared.state == nil {
		return ErrPreparedIndexedInvalid
	}
	state, ok := prepared.state.(*preparedIndexedState)
	if !ok || state == nil || state.core == nil {
		return ErrPreparedIndexedInvalid
	}
	if state.desc.IndexBuffer != p.indexBuffer || state.desc.IndexFormat != p.indexBufferFormat || state.desc.IndexOffset != p.indexBufferOffset {
		return ErrPreparedIndexedState
	}
	if err := p.core.ExecutePreparedIndexedCommands(state.core); err != nil {
		if errors.Is(err, hal.ErrPreparedIndexedState) {
			return ErrPreparedIndexedAlreadyExecuted
		}
		return mapPreparedIndexedError(err)
	}
	return nil
}
