//go:build js && wasm

package wgpu

// MaterialPage is unavailable in browser WebGPU; the type remains present so
// shared renderer code can fail closed without build-tag branches.
type MaterialPage struct{}

func (d *Device) MaterialPageCapabilities() MaterialPageCapabilities {
	return MaterialPageCapabilities{}
}

func (p *RenderPipeline) NewMaterialPage(_ *TextureView, _ *Sampler) (*MaterialPage, error) {
	return nil, ErrMaterialPageUnsupported
}

func (p *MaterialPage) Release() {}

func (p *RenderPassEncoder) SetMaterialPage(_ *MaterialPage) error { return ErrMaterialPageUnsupported }
