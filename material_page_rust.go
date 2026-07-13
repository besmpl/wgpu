//go:build rust

package wgpu

// MaterialPage is unavailable in the Rust FFI backend. Keep a typed stub so
// callers can use the same capability-gated path on every build surface.
type MaterialPage struct{}

func (d *Device) MaterialPageCapabilities() MaterialPageCapabilities {
	return MaterialPageCapabilities{}
}

func (p *RenderPipeline) NewMaterialPage(_ *TextureView, _ *Sampler) (*MaterialPage, error) {
	return nil, ErrMaterialPageUnsupported
}

func (p *MaterialPage) Release() {}

func (p *RenderPassEncoder) SetMaterialPage(_ *MaterialPage) error { return ErrMaterialPageUnsupported }
