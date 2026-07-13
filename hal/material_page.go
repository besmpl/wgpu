//go:build !(js && wasm)

package hal

// MaterialPageCapabilities describes the optional, fixed material-page ABI
// exposed by a backend. Implementations that do not support the ABI simply
// omit MaterialPageProvider.
type MaterialPageCapabilities struct {
	Supported           bool
	ABIVersion          uint32
	FragmentBufferIndex uint32
}

// MaterialPage is an opaque, immutable backend page. Its implementation owns
// any native argument buffer and child resources until Destroy is called.
type MaterialPage interface {
	Resource
}

// MaterialPageProvider is an optional device extension. It deliberately
// combines capability reporting and factory creation so top-level pipeline
// creation has one legal route to backend allocation.
type MaterialPageProvider interface {
	MaterialPageCapabilities() MaterialPageCapabilities
	CreateMaterialPage(pipeline RenderPipeline, view TextureView, sampler Sampler) (MaterialPage, error)
}

// MaterialPageRenderPassEncoder is an optional render-pass extension used to
// bind one immutable page as inherited pass state.
type MaterialPageRenderPassEncoder interface {
	SetMaterialPage(page MaterialPage) error
}
