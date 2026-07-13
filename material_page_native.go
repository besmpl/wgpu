//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"
	"sync/atomic"

	"github.com/besmpl/wgpu/core"
	"github.com/besmpl/wgpu/hal"
)

// MaterialPage is an immutable pipeline-bound page. Copies share the same
// reference-counted native ownership and Release is idempotent.
type MaterialPage struct {
	hal                 hal.MaterialPage
	device              *Device
	pipelineFingerprint uint64
	view                *TextureView
	sampler             *Sampler
	released            *atomic.Bool
	ref                 *core.ResourceRef
}

// MaterialPageCapabilities reports whether the current device exposes the
// optional page ABI.
func (d *Device) MaterialPageCapabilities() MaterialPageCapabilities {
	if d == nil || d.released.Load() {
		return MaterialPageCapabilities{}
	}
	provider, ok := d.halDevice().(hal.MaterialPageProvider)
	if !ok {
		return MaterialPageCapabilities{}
	}
	caps := provider.MaterialPageCapabilities()
	return MaterialPageCapabilities{Supported: caps.Supported, ABIVersion: caps.ABIVersion, FragmentBufferIndex: caps.FragmentBufferIndex}
}

// NewMaterialPage creates an immutable page from a marked specialized
// pipeline and one same-device texture view/sampler pair.
func (p *RenderPipeline) NewMaterialPage(view *TextureView, sampler *Sampler) (*MaterialPage, error) {
	if p == nil || p.device == nil || p.released {
		return nil, ErrReleased
	}
	if p.materialPage == nil || !p.materialPage.valid() {
		return nil, ErrMaterialPageUnsupported
	}
	if view == nil || sampler == nil || view.released || sampler.released || view.device != p.device || sampler.device != p.device {
		return nil, ErrMaterialPageOwnership
	}
	provider, ok := p.device.halDevice().(hal.MaterialPageProvider)
	if !ok {
		return nil, ErrMaterialPageUnsupported
	}
	caps := provider.MaterialPageCapabilities()
	if !caps.Supported || caps.ABIVersion != p.materialPage.ABIVersion || caps.FragmentBufferIndex != p.materialPage.FragmentBufferIndex {
		return nil, ErrMaterialPageUnsupported
	}
	hp, err := provider.CreateMaterialPage(p.hal, view.hal, sampler.hal)
	if err != nil {
		return nil, err
	}
	if hp == nil {
		return nil, ErrMaterialPageInvalid
	}
	page := &MaterialPage{hal: hp, device: p.device, pipelineFingerprint: p.materialPageFingerprint, view: view, sampler: sampler, released: new(atomic.Bool)}
	page.ref = core.NewResourceRef("MaterialPage", func() {
		if page.hal != nil {
			page.hal.Destroy()
			page.hal = nil
		}
	})
	return page, nil
}

// Release releases the caller's page reference. A page bound in a command
// encoder remains alive through submission via the existing ResourceRef path.
func (p *MaterialPage) Release() {
	if p == nil || p.isReleased() {
		return
	}
	if p.released == nil {
		p.released = new(atomic.Bool)
	}
	if !p.released.CompareAndSwap(false, true) {
		return
	}
	if p.ref != nil {
		p.ref.Drop()
	}
}

func (p *MaterialPage) matches(pipeline *RenderPipeline) bool {
	return p != nil && !p.isReleased() && pipeline != nil && pipeline.materialPage != nil &&
		p.pipelineFingerprint == pipeline.materialPageFingerprint
}

func (p *RenderPassEncoder) SetMaterialPage(page *MaterialPage) error {
	if page == nil {
		if p != nil && p.encoder != nil {
			p.encoder.setError(ErrMaterialPageInvalid)
		}
		return ErrMaterialPageInvalid
	}
	if p.encoder == nil || p.core == nil || !p.pipelineSet {
		if p.encoder != nil {
			p.encoder.setError(ErrMaterialPageInactivePass)
		}
		return ErrMaterialPageInactivePass
	}
	if page.isReleased() {
		p.encoder.setError(ErrMaterialPageReleased)
		return ErrMaterialPageReleased
	}
	if page.device != p.encoder.device {
		p.encoder.setError(ErrMaterialPageOwnership)
		return ErrMaterialPageOwnership
	}
	if !page.matches(p.currentPipeline) {
		p.encoder.setError(ErrMaterialPagePipelineMismatch)
		return ErrMaterialPagePipelineMismatch
	}
	raw := p.core.RawPass()
	binder, ok := raw.(hal.MaterialPageRenderPassEncoder)
	if !ok {
		p.encoder.setError(ErrMaterialPageUnsupported)
		return ErrMaterialPageUnsupported
	}
	if err := binder.SetMaterialPage(page.hal); err != nil {
		p.encoder.setError(err)
		return err
	}
	p.materialPage = page
	if p.currentPipeline.materialPage != nil {
		p.binder.assignMaterialPage(p.currentPipeline.materialPage.BindGroupIndex)
	}
	p.trackRef(page.ref)
	p.encoder.trackMaterialPage(page)
	if page.view != nil && page.view.texture != nil {
		p.encoder.trackTexture(page.view.texture)
	}
	return nil
}

func (p *MaterialPage) isReleased() bool {
	return p != nil && p.released != nil && p.released.Load()
}

func materialPageDrawError() error {
	return fmt.Errorf("%w: call SetMaterialPage after SetPipeline", ErrMaterialPagePipelineMismatch)
}
