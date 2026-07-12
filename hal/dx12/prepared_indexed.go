//go:build windows && !(js && wasm)

package dx12

import (
	"fmt"

	"github.com/gogpu/wgpu/hal"
)

type preparedIndexedCommands struct{ batch hal.IndexedBatchDescriptor }

func (e *CommandEncoder) PrepareIndexed(batch hal.IndexedBatchDescriptor) (hal.PreparedIndexedCommands, error) {
	if e == nil || !e.isRecording {
		return nil, fmt.Errorf("dx12: command encoder is not recording")
	}
	if batch.Arguments == nil || batch.IndexBuffer == nil || batch.Count == 0 {
		return nil, fmt.Errorf("dx12: invalid prepared indexed batch")
	}
	return &preparedIndexedCommands{batch: batch}, nil
}

func (e *RenderPassEncoder) ExecutePreparedIndexed(prepared hal.PreparedIndexedCommands) error {
	token, ok := prepared.(*preparedIndexedCommands)
	if !ok || token == nil {
		return fmt.Errorf("dx12: invalid prepared indexed token")
	}
	return e.executePreparedIndexed(token.batch.Arguments, token.batch.ArgumentsOffset, token.batch.Count)
}

func (d *Device) PreparedIndexedCapabilities() hal.PreparedIndexedCapabilities {
	return hal.PreparedIndexedCapabilities{PreparedIndexed: true, MaxCommands: 1 << 20}
}

func (*Device) PreparedIndexedRequiresFeature() bool { return false }

var _ hal.PreparedIndexedCommandEncoder = (*CommandEncoder)(nil)
var _ hal.PreparedIndexedRenderPass = (*RenderPassEncoder)(nil)
var _ hal.PreparedIndexedCapabilityProvider = (*Device)(nil)
