//go:build rust

package wgpu

// PreparedIndexedCapabilities always reports unsupported for the Rust FFI
// surface until the upstream wrapper exposes the encoder/HAL seam.
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
