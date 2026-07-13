//go:build !(js && wasm)

package vulkan

import (
	"fmt"

	"github.com/besmpl/wgpu/hal"
)

type preparedIndexedCommands struct{ batch hal.IndexedBatchDescriptor }

func (e *CommandEncoder) PrepareIndexed(batch hal.IndexedBatchDescriptor) (hal.PreparedIndexedCommands, error) {
	if e == nil || e.active == 0 {
		return nil, fmt.Errorf("vulkan: command encoder is not recording")
	}
	if batch.Arguments == nil || batch.IndexBuffer == nil || batch.Count == 0 {
		return nil, fmt.Errorf("vulkan: invalid prepared indexed batch")
	}
	return &preparedIndexedCommands{batch: batch}, nil
}

func (e *RenderPassEncoder) ExecutePreparedIndexed(prepared hal.PreparedIndexedCommands) error {
	token, ok := prepared.(*preparedIndexedCommands)
	if !ok || token == nil {
		return fmt.Errorf("vulkan: invalid prepared indexed token")
	}
	return e.executePreparedIndexed(token.batch.Arguments, token.batch.ArgumentsOffset, token.batch.Count)
}

func (d *Device) PreparedIndexedCapabilities() hal.PreparedIndexedCapabilities {
	if d == nil || d.maxDrawIndirectCount == 0 {
		return hal.PreparedIndexedCapabilities{}
	}
	return hal.PreparedIndexedCapabilities{PreparedIndexed: true, MaxCommands: d.maxDrawIndirectCount}
}

func (*Device) PreparedIndexedRequiresFeature() bool { return true }

var _ hal.PreparedIndexedCommandEncoder = (*CommandEncoder)(nil)
var _ hal.PreparedIndexedRenderPass = (*RenderPassEncoder)(nil)
var _ hal.PreparedIndexedCapabilityProvider = (*Device)(nil)
