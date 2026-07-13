//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/besmpl/wgpu/core"
	"github.com/gogpu/gputypes"
)

func testMaterialPage() (*MaterialPage, *int32) {
	var destroyed int32
	page := &MaterialPage{}
	page.ref = core.NewResourceRef("test-material-page", func() { atomic.AddInt32(&destroyed, 1) })
	page.released = new(atomic.Bool)
	return page, &destroyed
}

func TestMaterialPageReleaseIsIdempotent(t *testing.T) {
	page, destroyed := testMaterialPage()
	copyPage := *page
	copyPage.Release()
	page.Release()
	if got := atomic.LoadInt32(destroyed); got != 1 {
		t.Fatalf("page release callback count = %d, want 1", got)
	}
}

func TestMaterialPageBindRequiresActivePassAndPipeline(t *testing.T) {
	page, _ := testMaterialPage()
	pass := &RenderPassEncoder{encoder: &CommandEncoder{}}
	if err := pass.SetMaterialPage(page); !errors.Is(err, ErrMaterialPageInactivePass) {
		t.Fatalf("SetMaterialPage without active pass = %v, want %v", err, ErrMaterialPageInactivePass)
	}
}

func TestMaterialPageBindNilIsSafe(t *testing.T) {
	var pass RenderPassEncoder
	if err := pass.SetMaterialPage(nil); !errors.Is(err, ErrMaterialPageInvalid) {
		t.Fatalf("zero pass SetMaterialPage(nil) = %v, want %v", err, ErrMaterialPageInvalid)
	}
	var nilPass *RenderPassEncoder
	if err := nilPass.SetMaterialPage(nil); !errors.Is(err, ErrMaterialPageInvalid) {
		t.Fatalf("nil pass SetMaterialPage(nil) = %v, want %v", err, ErrMaterialPageInvalid)
	}
}

func TestMaterialPageLayoutRequiresEmptyMarkedGroup(t *testing.T) {
	page := &MaterialPageDescriptor{ABIVersion: 1, BindGroupIndex: 0, TextureArgumentIndex: 0, SamplerArgumentIndex: 1}
	if err := validateMaterialPageLayout(&RenderPipelineDescriptor{MaterialPage: page}); !errors.Is(err, ErrMaterialPageInvalid) {
		t.Fatalf("missing marked group layout = %v, want %v", err, ErrMaterialPageInvalid)
	}
	nonempty := &BindGroupLayout{entries: []gputypes.BindGroupLayoutEntry{{Binding: 0}}}
	layout := &PipelineLayout{bindGroupLayouts: []*BindGroupLayout{nonempty}}
	if err := validateMaterialPageLayout(&RenderPipelineDescriptor{Layout: layout, MaterialPage: page}); !errors.Is(err, ErrMaterialPageInvalid) {
		t.Fatalf("nonempty marked group layout = %v, want %v", err, ErrMaterialPageInvalid)
	}
}

func TestMaterialPageBindRejectsMismatchedPipelineBeforeHAL(t *testing.T) {
	page, _ := testMaterialPage()
	page.pipelineFingerprint = 1
	pass := &RenderPassEncoder{encoder: &CommandEncoder{}, core: &core.CoreRenderPassEncoder{}, pipelineSet: true,
		currentPipeline: &RenderPipeline{materialPage: &MaterialPageDescriptor{ABIVersion: 1, TextureArgumentIndex: 0, SamplerArgumentIndex: 1}, materialPageFingerprint: 2}}
	if err := pass.SetMaterialPage(page); !errors.Is(err, ErrMaterialPagePipelineMismatch) {
		t.Fatalf("mismatched page bind = %v, want %v", err, ErrMaterialPagePipelineMismatch)
	}
}
