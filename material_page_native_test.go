//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/besmpl/wgpu/core"
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

func TestMaterialPageBindRejectsMismatchedPipelineBeforeHAL(t *testing.T) {
	page, _ := testMaterialPage()
	page.pipelineFingerprint = 1
	pass := &RenderPassEncoder{encoder: &CommandEncoder{}, core: &core.CoreRenderPassEncoder{}, pipelineSet: true,
		currentPipeline: &RenderPipeline{materialPage: &MaterialPageDescriptor{ABIVersion: 1, TextureArgumentIndex: 0, SamplerArgumentIndex: 1}, materialPageFingerprint: 2}}
	if err := pass.SetMaterialPage(page); !errors.Is(err, ErrMaterialPagePipelineMismatch) {
		t.Fatalf("mismatched page bind = %v, want %v", err, ErrMaterialPagePipelineMismatch)
	}
}
