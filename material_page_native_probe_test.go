//go:build darwin && !rust && !(js && wasm) && hearth_material_page_probe

package wgpu

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/besmpl/wgpu/hal/allbackends"
	"github.com/gogpu/gputypes"
)

// TestMaterialPageMetalICBProbe is an opt-in physical proof for the marked
// explicit-MSL ABI. It intentionally uses the public WGPU operations so the
// native page provider, ICB inheritance, residency, and tracked lifetime are
// exercised together.
func TestMaterialPageMetalICBProbe(t *testing.T) {
	if os.Getenv("HEARTH_MATERIAL_PAGE_PROBE") != "1" {
		t.Skip("set HEARTH_MATERIAL_PAGE_PROBE=1 to run physical Metal proof")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	instance, err := CreateInstance(&InstanceDescriptor{Backends: BackendsMetal})
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	defer instance.Release()
	adapter, err := instance.RequestAdapter(&RequestAdapterOptions{PowerPreference: PowerPreferenceHighPerformance})
	if err != nil || adapter == nil {
		t.Skipf("Metal adapter unavailable: %v", err)
	}
	defer adapter.Release()
	if !strings.EqualFold(adapter.Info().Backend.String(), "Metal") {
		t.Fatalf("selected backend %s", adapter.Info().Backend)
	}
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		t.Fatalf("request device: %v", err)
	}
	defer device.Release()
	if caps := device.MaterialPageCapabilities(); !caps.Supported || caps.ABIVersion != 1 || caps.FragmentBufferIndex != 7 {
		t.Fatalf("material-page capability unavailable: %+v", caps)
	}
	queue := device.Queue()

	const msl = `#include <metal_stdlib>
using namespace metal;
struct MaterialPageArgs { texture2d_array<float> page [[id(0)]]; sampler samp [[id(1)]]; };
struct VSIn { float2 position [[attribute(0)]]; };
struct VSOut { float4 position [[position]]; uint layer [[flat]]; };
vertex VSOut vs(VSIn in [[stage_in]], uint instance_id [[instance_id]], uint base_instance [[base_instance]]) {
  VSOut out; out.position = float4(in.position, 0.0, 1.0); out.layer = base_instance; return out;
}
fragment float4 fs(VSOut in [[stage_in]], constant MaterialPageArgs& args [[buffer(7)]]) {
	float4 direct = args.page.read(uint2(0, 0), in.layer, 0);
	float4 filtered = args.page.sample(args.samp, float2(0.5, 0.5), in.layer);
	return direct + filtered * 0.000001;
}`
	abi := &MaterialPageShaderDescriptor{ABIVersion: 1, BindGroupIndex: 0, TextureBinding: 0, SamplerBinding: 1, TextureArgumentIndex: 0, SamplerArgumentIndex: 1, FragmentBufferIndex: 7}
	shader, err := device.CreateShaderModule(&ShaderModuleDescriptor{Label: "material page probe shader", MSL: msl, MaterialPage: abi})
	if err != nil {
		t.Fatalf("create explicit MSL shader: %v", err)
	}
	defer shader.Release()
	desc := &RenderPipelineDescriptor{Label: "material page probe flagged", MaterialPage: (*MaterialPageDescriptor)(abi), SupportIndirectCommandBuffers: true,
		Vertex:    VertexState{Module: shader, EntryPoint: "vs", Buffers: []gputypes.VertexBufferLayout{{ArrayStride: 8, StepMode: gputypes.VertexStepModeVertex, Attributes: []gputypes.VertexAttribute{{Format: gputypes.VertexFormatFloat32x2, Offset: 0, ShaderLocation: 0}}}}},
		Primitive: gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList}, Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xffffffff},
		Fragment: &FragmentState{Module: shader, EntryPoint: "fs", Targets: []ColorTargetState{{Format: TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll}}}}
	emptyLayout, err := device.CreateBindGroupLayout(&BindGroupLayoutDescriptor{Label: "material page empty group"})
	if err != nil {
		t.Fatalf("create material page empty bind-group layout: %v", err)
	}
	defer emptyLayout.Release()
	pipelineLayout, err := device.CreatePipelineLayout(&PipelineLayoutDescriptor{Label: "material page probe layout", BindGroupLayouts: []*BindGroupLayout{emptyLayout}})
	if err != nil {
		t.Fatalf("create material page pipeline layout: %v", err)
	}
	defer pipelineLayout.Release()
	desc.Layout = pipelineLayout
	flagged, err := device.CreateRenderPipeline(desc)
	if err != nil {
		t.Fatalf("create flagged pipeline: %v", err)
	}
	defer flagged.Release()
	unflaggedDesc := *desc
	unflaggedDesc.SupportIndirectCommandBuffers = false
	unflaggedDesc.Label = "material page probe individual"
	unflagged, err := device.CreateRenderPipeline(&unflaggedDesc)
	if err != nil {
		t.Fatalf("create unflagged pipeline: %v", err)
	}
	defer unflagged.Release()

	pageTexture, err := device.CreateTexture(&TextureDescriptor{Label: "material page layers", Size: Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 2}, MipLevelCount: 1, SampleCount: 1, Dimension: TextureDimension2D, Format: TextureFormatRGBA8Unorm, Usage: TextureUsageTextureBinding | TextureUsageCopyDst})
	if err != nil {
		t.Fatalf("create page texture: %v", err)
	}
	defer pageTexture.Release()
	pageView, err := device.CreateTextureView(pageTexture, &TextureViewDescriptor{Dimension: gputypes.TextureViewDimension2DArray, ArrayLayerCount: 2})
	if err != nil {
		t.Fatalf("create page view: %v", err)
	}
	defer pageView.Release()
	for layer, pixel := range [][]byte{{255, 0, 0, 255}, {0, 255, 0, 255}} {
		if err := queue.WriteTexture(&ImageCopyTexture{Texture: pageTexture, Origin: Origin3D{Z: uint32(layer)}}, pixel, &ImageDataLayout{BytesPerRow: 4, RowsPerImage: 1}, &Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1}); err != nil {
			t.Fatalf("upload page layer %d: %v", layer, err)
		}
	}
	sampler, err := device.CreateSampler(&SamplerDescriptor{AddressModeU: gputypes.AddressModeClampToEdge, AddressModeV: gputypes.AddressModeClampToEdge, AddressModeW: gputypes.AddressModeClampToEdge, MagFilter: gputypes.FilterModeNearest, MinFilter: gputypes.FilterModeNearest, MipmapFilter: gputypes.FilterModeNearest})
	if err != nil {
		t.Fatalf("create page sampler: %v", err)
	}
	defer sampler.Release()
	page, err := flagged.NewMaterialPage(pageView, sampler)
	if err != nil {
		t.Fatalf("create material page: %v", err)
	}

	vertex, err := device.CreateBuffer(&BufferDescriptor{Size: 64, Usage: BufferUsageVertex | BufferUsageCopyDst})
	if err != nil {
		t.Fatal(err)
	}
	defer vertex.Release()
	verts := []float32{-1, -1, 0, -1, -1, 1, 0, 1, 0, -1, 1, -1, 0, 1, 1, 1}
	vbytes := make([]byte, len(verts)*4)
	for i, v := range verts {
		binary.LittleEndian.PutUint32(vbytes[i*4:], mathFloat32bits(v))
	}
	if err := queue.WriteBuffer(vertex, 0, vbytes); err != nil {
		t.Fatal(err)
	}
	index, err := device.CreateBuffer(&BufferDescriptor{Size: 48, Usage: BufferUsageIndex | BufferUsageCopyDst})
	if err != nil {
		t.Fatal(err)
	}
	defer index.Release()
	ibytes := make([]byte, 48)
	for i, v := range []uint32{0, 1, 2, 2, 1, 3, 0, 1, 2, 2, 1, 3} {
		binary.LittleEndian.PutUint32(ibytes[i*4:], v)
	}
	if err := queue.WriteBuffer(index, 0, ibytes); err != nil {
		t.Fatal(err)
	}
	indirect, err := device.CreateBuffer(&BufferDescriptor{Size: 40, Usage: BufferUsageIndirect | BufferUsageStorage | BufferUsageCopyDst})
	if err != nil {
		t.Fatal(err)
	}
	defer indirect.Release()
	records := make([]byte, 40)
	binary.LittleEndian.PutUint32(records[0:], 6)
	binary.LittleEndian.PutUint32(records[4:], 1)
	binary.LittleEndian.PutUint32(records[20:], 6)
	binary.LittleEndian.PutUint32(records[24:], 1)
	binary.LittleEndian.PutUint32(records[28:], 6)
	binary.LittleEndian.PutUint32(records[32:], 4)
	binary.LittleEndian.PutUint32(records[36:], 1)
	if err := queue.WriteBuffer(indirect, 0, records); err != nil {
		t.Fatal(err)
	}

	prepared := renderMaterialPageFrame(t, device, queue, flagged, page, vertex, index, indirect)
	preparedSecond := renderMaterialPageFrame(t, device, queue, flagged, page, vertex, index, indirect)
	if !bytes.Equal(prepared, preparedSecond) {
		t.Fatal("consecutive prepared page frames differ")
	}
	oracle := renderMaterialPageOracle(t, device, queue, unflagged, page, vertex, index, indirect)
	if !bytes.Equal(prepared, oracle) {
		t.Fatalf("prepared page readback differs from individual page draws (prepared=%x oracle=%x)", prepared[:minInt(32, len(prepared))], oracle[:minInt(32, len(oracle))])
	}
	if !hasRedGreen(prepared) {
		t.Fatalf("page readback did not contain both layer colors: %x", prepared[:minInt(64, len(prepared))])
	}
	if err := testMaterialPageSubmitRejectsReleased(t, device, queue, flagged, pageView, sampler, vertex, index, indirect); !errors.Is(err, ErrMaterialPageReleased) {
		t.Fatalf("release-before-submit error = %v", err)
	}
	testMaterialPageReleaseAfterSubmit(t, device, queue, flagged, vertex, index, indirect)
	// The page's retained fragment function/argument buffer must survive its
	// source pipeline release; release-before-page is deliberately exercised.
	flagged.Release()
	postRelease := renderMaterialPageOracle(t, device, queue, unflagged, page, vertex, index, indirect)
	if !bytes.Equal(prepared, postRelease) {
		t.Fatal("page became invalid after creator pipeline release")
	}
	page.Release()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func mathFloat32bits(v float32) uint32 { return math.Float32bits(v) }

func hasRedGreen(data []byte) bool {
	var red, green bool
	for i := 0; i+3 < len(data); i += 4 {
		red = red || data[i] > 200 && data[i+1] < 40
		green = green || data[i+1] > 200 && data[i] < 40
	}
	return red && green
}

func renderMaterialPageFrame(t *testing.T, device *Device, queue *Queue, pipeline *RenderPipeline, page *MaterialPage, vertex, index, indirect *Buffer) []byte {
	return renderMaterialPage(t, device, queue, pipeline, page, vertex, index, indirect, true)
}
func renderMaterialPageOracle(t *testing.T, device *Device, queue *Queue, pipeline *RenderPipeline, page *MaterialPage, vertex, index, indirect *Buffer) []byte {
	return renderMaterialPage(t, device, queue, pipeline, page, vertex, index, indirect, false)
}

func renderMaterialPage(t *testing.T, device *Device, queue *Queue, pipeline *RenderPipeline, page *MaterialPage, vertex, index, indirect *Buffer, prepared bool) []byte {
	target, err := device.CreateTexture(&TextureDescriptor{Size: Extent3D{Width: 4, Height: 2, DepthOrArrayLayers: 1}, MipLevelCount: 1, SampleCount: 1, Dimension: TextureDimension2D, Format: TextureFormatRGBA8Unorm, Usage: TextureUsageRenderAttachment | TextureUsageCopySrc})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Release()
	view, err := device.CreateTextureView(target, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer view.Release()
	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	var token *PreparedIndexedCommands
	if prepared {
		token, err = enc.PrepareIndexedCommands(PreparedIndexedCommandsDescriptor{Arguments: indirect, Count: 2, IndexBuffer: index, IndexFormat: gputypes.IndexFormatUint32})
		if err != nil {
			t.Fatal(err)
		}
	}
	pass, err := enc.BeginRenderPass(&RenderPassDescriptor{ColorAttachments: []RenderPassColorAttachment{{View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore}}})
	if err != nil {
		t.Fatal(err)
	}
	pass.SetPipeline(pipeline)
	if err := pass.SetMaterialPage(page); err != nil {
		t.Fatal(err)
	}
	pass.SetVertexBuffer(0, vertex, 0)
	pass.SetIndexBuffer(index, gputypes.IndexFormatUint32, 0)
	if prepared {
		if err := pass.ExecutePreparedIndexedCommands(token); err != nil {
			t.Fatal(err)
		}
	} else {
		pass.DrawIndexed(6, 1, 0, 0, 0)
		pass.DrawIndexed(6, 1, 6, 4, 1)
	}
	if err := pass.End(); err != nil {
		t.Fatal(err)
	}
	readback, err := device.CreateBuffer(&BufferDescriptor{Size: 2048, Usage: BufferUsageCopyDst | BufferUsageMapRead})
	if err != nil {
		t.Fatal(err)
	}
	defer readback.Release()
	enc.CopyTextureToBuffer(target, readback, []BufferTextureCopy{{TextureBase: ImageCopyTexture{Texture: target}, BufferLayout: ImageDataLayout{BytesPerRow: 256, RowsPerImage: 2}, Size: Extent3D{Width: 4, Height: 2, DepthOrArrayLayers: 1}}})
	cb, err := enc.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Submit(cb); err != nil {
		t.Fatal(err)
	}
	device.Poll(PollWait)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := readback.Map(ctx, MapModeRead, 0, 2048); err != nil {
		t.Fatal(err)
	}
	r, err := readback.MappedRange(0, 2048)
	if err != nil {
		t.Fatal(err)
	}
	data := append([]byte(nil), r.Bytes()...)
	if err := readback.Unmap(); err != nil {
		t.Fatal(err)
	}
	return data
}

func testMaterialPageSubmitRejectsReleased(t *testing.T, device *Device, queue *Queue, pipeline *RenderPipeline, view *TextureView, sampler *Sampler, vertex, index, indirect *Buffer) error {
	page, err := pipeline.NewMaterialPage(view, sampler)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	target, err := device.CreateTexture(&TextureDescriptor{Size: Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 1}, MipLevelCount: 1, SampleCount: 1, Dimension: TextureDimension2D, Format: TextureFormatRGBA8Unorm, Usage: TextureUsageRenderAttachment})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Release()
	targetView, err := device.CreateTextureView(target, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer targetView.Release()
	pass, err := enc.BeginRenderPass(&RenderPassDescriptor{ColorAttachments: []RenderPassColorAttachment{{View: targetView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore}}})
	if err != nil {
		t.Fatal(err)
	}
	pass.SetPipeline(pipeline)
	if err := pass.SetMaterialPage(page); err != nil {
		t.Fatal(err)
	}
	pass.SetVertexBuffer(0, vertex, 0)
	pass.SetIndexBuffer(index, gputypes.IndexFormatUint32, 0)
	pass.DrawIndexedIndirect(indirect, 0)
	if err := pass.End(); err != nil {
		t.Fatal(err)
	}
	cb, err := enc.Finish()
	if err != nil {
		t.Fatal(err)
	}
	page.Release()
	_, err = queue.Submit(cb)
	cb.Release()
	return err
}

// testMaterialPageReleaseAfterSubmit proves that command-buffer tracking keeps
// the page and its source view/sampler/root texture alive until the GPU work is
// complete. The releases intentionally happen immediately after Submit and
// before PollWait.
func testMaterialPageReleaseAfterSubmit(t *testing.T, device *Device, queue *Queue, pipeline *RenderPipeline, vertex, index, indirect *Buffer) {
	pageTexture, err := device.CreateTexture(&TextureDescriptor{Label: "material page deferred texture", Size: Extent3D{Width: 1, Height: 1, DepthOrArrayLayers: 2}, MipLevelCount: 1, SampleCount: 1, Dimension: TextureDimension2D, Format: TextureFormatRGBA8Unorm, Usage: TextureUsageTextureBinding | TextureUsageCopyDst})
	if err != nil {
		t.Fatal(err)
	}
	pageView, err := device.CreateTextureView(pageTexture, &TextureViewDescriptor{Dimension: gputypes.TextureViewDimension2DArray, ArrayLayerCount: 2})
	if err != nil {
		pageTexture.Release()
		t.Fatal(err)
	}
	sampler, err := device.CreateSampler(&SamplerDescriptor{AddressModeU: gputypes.AddressModeClampToEdge, AddressModeV: gputypes.AddressModeClampToEdge, AddressModeW: gputypes.AddressModeClampToEdge, MagFilter: gputypes.FilterModeNearest, MinFilter: gputypes.FilterModeNearest, MipmapFilter: gputypes.FilterModeNearest})
	if err != nil {
		pageView.Release()
		pageTexture.Release()
		t.Fatal(err)
	}
	page, err := pipeline.NewMaterialPage(pageView, sampler)
	if err != nil {
		sampler.Release()
		pageView.Release()
		pageTexture.Release()
		t.Fatal(err)
	}
	target, err := device.CreateTexture(&TextureDescriptor{Label: "material page deferred target", Size: Extent3D{Width: 4, Height: 2, DepthOrArrayLayers: 1}, MipLevelCount: 1, SampleCount: 1, Dimension: TextureDimension2D, Format: TextureFormatRGBA8Unorm, Usage: TextureUsageRenderAttachment})
	if err != nil {
		page.Release()
		sampler.Release()
		pageView.Release()
		pageTexture.Release()
		t.Fatal(err)
	}
	targetView, err := device.CreateTextureView(target, nil)
	if err != nil {
		target.Release()
		page.Release()
		sampler.Release()
		pageView.Release()
		pageTexture.Release()
		t.Fatal(err)
	}
	enc, err := device.CreateCommandEncoder(nil)
	if err != nil {
		targetView.Release()
		target.Release()
		page.Release()
		sampler.Release()
		pageView.Release()
		pageTexture.Release()
		t.Fatal(err)
	}
	pass, err := enc.BeginRenderPass(&RenderPassDescriptor{ColorAttachments: []RenderPassColorAttachment{{View: targetView, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore}}})
	if err != nil {
		targetView.Release()
		target.Release()
		page.Release()
		sampler.Release()
		pageView.Release()
		pageTexture.Release()
		t.Fatal(err)
	}
	pass.SetPipeline(pipeline)
	if err := pass.SetMaterialPage(page); err != nil {
		t.Fatal(err)
	}
	pass.SetVertexBuffer(0, vertex, 0)
	pass.SetIndexBuffer(index, gputypes.IndexFormatUint32, 0)
	pass.DrawIndexedIndirect(indirect, 0)
	if err := pass.End(); err != nil {
		t.Fatal(err)
	}
	cb, err := enc.Finish()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Submit(cb); err != nil {
		cb.Release()
		t.Fatal(err)
	}
	// These are intentionally released before the completion wait. Submit's
	// tracked refs must keep all page dependencies usable until PollWait.
	page.Release()
	pageView.Release()
	sampler.Release()
	pageTexture.Release()
	device.Poll(PollWait)
	targetView.Release()
	target.Release()
}
