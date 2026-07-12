//go:build js && wasm

package wgpu

// PreparedIndexedCapabilities always reports unsupported for browser builds:
// the browser WebGPU surface has no encoder-time preparation or ICB bridge.
func (d *Device) PreparedIndexedCapabilities() PreparedIndexedCapabilities {
	return PreparedIndexedCapabilities{}
}

func (e *CommandEncoder) PrepareIndexedCommands(_ PreparedIndexedCommandsDescriptor) (*PreparedIndexedCommands, error) {
	if e == nil || e.released {
		return nil, ErrReleased
	}
	return nil, ErrPreparedIndexedUnsupported
}

func (p *RenderPassEncoder) ExecutePreparedIndexedCommands(_ *PreparedIndexedCommands) error {
	if p == nil || p.released {
		return ErrReleased
	}
	return ErrPreparedIndexedUnsupported
}
