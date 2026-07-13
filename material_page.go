package wgpu

import "errors"

// MaterialPageCapabilities describes the optional, fixed ABI used by the
// Metal material-page path. A false Supported value is the normal result on
// backends that do not expose the private extension.
type MaterialPageCapabilities struct {
	Supported           bool
	ABIVersion          uint32
	FragmentBufferIndex uint32
}

// MaterialPageDescriptor identifies the marked shader ABI consumed by a
// material page. The descriptor is immutable pipeline metadata; callers never
// manipulate argument buffers or native resources directly.
type MaterialPageDescriptor struct {
	ABIVersion           uint32
	BindGroupIndex       uint32
	TextureBinding       uint32
	SamplerBinding       uint32
	TextureArgumentIndex uint32
	SamplerArgumentIndex uint32
	FragmentBufferIndex  uint32
}

// MaterialPageShaderDescriptor marks the shader module carrying the material
// page ABI. It intentionally aliases the pipeline descriptor shape so the
// compiler and pipeline layout can validate one fixed contract.
type MaterialPageShaderDescriptor = MaterialPageDescriptor

var (
	ErrMaterialPageUnsupported      = errors.New("wgpu: material pages are unsupported")
	ErrMaterialPageInvalid          = errors.New("wgpu: invalid material page descriptor")
	ErrMaterialPageOwnership        = errors.New("wgpu: material page belongs to another device")
	ErrMaterialPageReleased         = errors.New("wgpu: material page is released")
	ErrMaterialPagePipelineMismatch = errors.New("wgpu: material page pipeline ABI mismatch")
	ErrMaterialPageInactivePass     = errors.New("wgpu: material page requires an active render pass")
)

func (d MaterialPageDescriptor) valid() bool {
	return d.ABIVersion != 0 && d.TextureArgumentIndex != d.SamplerArgumentIndex
}

func (d MaterialPageDescriptor) fingerprint() uint64 {
	// FNV-1a over the fixed-width ABI fields keeps compatibility deterministic
	// without exposing a hash implementation to callers.
	h := uint64(14695981039346656037)
	for _, v := range [...]uint32{
		d.ABIVersion, d.BindGroupIndex, d.TextureBinding, d.SamplerBinding,
		d.TextureArgumentIndex, d.SamplerArgumentIndex, d.FragmentBufferIndex,
	} {
		for i := uint(0); i < 4; i++ {
			h ^= uint64(byte(v >> (i * 8)))
			h *= 1099511628211
		}
	}
	return h
}
