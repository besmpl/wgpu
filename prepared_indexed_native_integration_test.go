//go:build !rust && !(js && wasm)

package wgpu_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"
)

// TestPreparedIndexedNativeParity is an opt-in, strict-backend proof of the
// public prepared indexed path. Two records exercise FirstIndex, BaseVertex,
// and FirstInstance; their readback must exactly match two ordinary indexed
// indirect draws using the same records.
func TestPreparedIndexedNativeParity(t *testing.T) {
	required := strings.ToLower(strings.TrimSpace(os.Getenv("WGPU_PREPARED_INDEXED_PROOF_BACKEND")))
	if required == "" {
		t.Skip("set WGPU_PREPARED_INDEXED_PROOF_BACKEND=vulkan or dx12 for native proof")
	}
	var backend wgpu.Backends
	switch required {
	case "vulkan":
		backend = wgpu.BackendsVulkan
	case "dx12":
		backend = wgpu.BackendsDX12
	default:
		t.Fatalf("unsupported WGPU_PREPARED_INDEXED_PROOF_BACKEND=%q (want vulkan or dx12)", required)
	}

	instance, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: backend})
	if err != nil {
		t.Fatalf("create %s instance: %v", required, err)
	}
	defer instance.Release()
	adapter, err := instance.RequestAdapter(&wgpu.RequestAdapterOptions{PowerPreference: wgpu.PowerPreferenceHighPerformance})
	if err != nil || adapter == nil {
		t.Fatalf("request %s adapter: %v", required, err)
	}
	defer adapter.Release()
	if got := strings.ToLower(adapter.Info().Backend.String()); got != required {
		t.Fatalf("strict backend selected %q, got %q on adapter %q", required, got, adapter.Info().Name)
	}

	optional := gputypes.Features(gputypes.FeatureMultiDrawIndirect | gputypes.FeatureIndirectFirstInstance)
	device, err := adapter.RequestDevice(&wgpu.DeviceDescriptor{RequiredFeatures: adapter.Features() & optional})
	if err != nil || device == nil {
		t.Fatalf("request %s device: %v", required, err)
	}
	defer device.Release()
	if caps := device.PreparedIndexedCapabilities(); !caps.PreparedIndexed || caps.MaxCommands < 2 {
		t.Fatalf("prepared indexed capability unavailable on %q: %+v", adapter.Info().Name, caps)
	}
	queue := device.Queue()
	if queue == nil {
		t.Fatal("device queue is nil")
	}

	shader, err := device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{Label: "prepared indexed parity shader", WGSL: `
struct VertexOutput {
  @builtin(position) position: vec4<f32>,
  @location(0) first_instance: u32,
};
@vertex
fn vs(@location(0) position: vec2<f32>, @builtin(instance_index) first_instance: u32) -> VertexOutput {
  var out: VertexOutput;
  out.position = vec4<f32>(position, 0.0, 1.0);
  out.first_instance = first_instance;
  return out;
}
@fragment
fn fs(@location(0) first_instance: u32) -> @location(0) vec4<f32> {
  if (first_instance == 0u) { return vec4<f32>(1.0, 0.0, 0.0, 1.0); }
  return vec4<f32>(0.0, 1.0, 0.0, 1.0);
}`})
	if err != nil {
		t.Fatalf("create parity shader: %v", err)
	}
	defer shader.Release()
	pipeline, err := device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label: "prepared indexed parity pipeline",
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "vs", Buffers: []gputypes.VertexBufferLayout{{
			ArrayStride: 8, StepMode: gputypes.VertexStepModeVertex,
			Attributes: []gputypes.VertexAttribute{{Format: gputypes.VertexFormatFloat32x2, Offset: 0, ShaderLocation: 0}},
		}}},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList, CullMode: gputypes.CullModeNone},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xffffffff},
		Fragment:    &wgpu.FragmentState{Module: shader, EntryPoint: "fs", Targets: []wgpu.ColorTargetState{{Format: wgpu.TextureFormatRGBA8Unorm, WriteMask: gputypes.ColorWriteMaskAll}}},
	})
	if err != nil {
		t.Fatalf("create parity pipeline: %v", err)
	}
	defer pipeline.Release()

	vertex, err := device.CreateBuffer(&wgpu.BufferDescriptor{Label: "prepared indexed vertices", Size: 48, Usage: wgpu.BufferUsageVertex | wgpu.BufferUsageCopyDst})
	if err != nil {
		t.Fatalf("create vertex buffer: %v", err)
	}
	defer vertex.Release()
	vertices := []float32{-1, -1, 0, -1, -1, 1, 0, 1, 1, -1, 1, 1}
	vertexBytes := make([]byte, len(vertices)*4)
	for i, value := range vertices {
		binary.LittleEndian.PutUint32(vertexBytes[i*4:], math.Float32bits(value))
	}
	if err := queue.WriteBuffer(vertex, 0, vertexBytes); err != nil {
		t.Fatalf("upload vertex buffer: %v", err)
	}

	index, err := device.CreateBuffer(&wgpu.BufferDescriptor{Label: "prepared indexed indices", Size: 24, Usage: wgpu.BufferUsageIndex | wgpu.BufferUsageCopyDst})
	if err != nil {
		t.Fatalf("create index buffer: %v", err)
	}
	defer index.Release()
	indexBytes := make([]byte, 6*4)
	for i, value := range []uint32{0, 1, 2, 0, 1, 2} {
		binary.LittleEndian.PutUint32(indexBytes[i*4:], value)
	}
	if err := queue.WriteBuffer(index, 0, indexBytes); err != nil {
		t.Fatalf("upload index buffer: %v", err)
	}

	indirect, err := device.CreateBuffer(&wgpu.BufferDescriptor{Label: "prepared indexed records", Size: 40, Usage: wgpu.BufferUsageIndirect | wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst})
	if err != nil {
		t.Fatalf("create indirect buffer: %v", err)
	}
	defer indirect.Release()
	records := make([]byte, 40)
	// indexCount, instanceCount, firstIndex, baseVertex, firstInstance.
	for i, value := range []uint32{3, 1, 0, 0, 0, 3, 1, 3, 3, 1} {
		binary.LittleEndian.PutUint32(records[i*4:], value)
	}
	if err := queue.WriteBuffer(indirect, 0, records); err != nil {
		t.Fatalf("upload indirect records: %v", err)
	}

	preparedPixels := recordPreparedTarget(t, device, queue, pipeline, vertex, index, indirect)
	oraclePixels := recordOracleTarget(t, device, queue, pipeline, vertex, index, indirect)
	assertParityColors(t, preparedPixels)
	if !bytes.Equal(preparedPixels, oraclePixels) {
		t.Fatal("prepared indexed readback differs from two individual indexed-indirect draws")
	}
}

func assertParityColors(t *testing.T, pixels []byte) {
	t.Helper()
	var red, green bool
	for row := 0; row < 4; row++ {
		for column := 0; column < 4; column++ {
			pixel := pixels[row*256+column*4:]
			red = red || pixel[0] > 200 && pixel[1] < 40
			green = green || pixel[1] > 200 && pixel[0] < 40
		}
	}
	if !red || !green {
		t.Fatalf("prepared indexed output colors red=%t green=%t; records were not both visibly consumed", red, green)
	}
}

func recordPreparedTarget(t *testing.T, device *wgpu.Device, queue *wgpu.Queue, pipeline *wgpu.RenderPipeline, vertex, index, indirect *wgpu.Buffer) []byte {
	t.Helper()
	target, view := newParityTarget(t, device, "prepared")
	defer target.Release()
	defer view.Release()
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("create prepared encoder: %v", err)
	}
	prepared, err := encoder.PrepareIndexedCommands(wgpu.PreparedIndexedCommandsDescriptor{Arguments: indirect, Count: 2, IndexBuffer: index, IndexFormat: gputypes.IndexFormatUint32})
	if err != nil {
		t.Fatalf("prepare indexed records: %v", err)
	}
	pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{ColorAttachments: []wgpu.RenderPassColorAttachment{{View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore}}})
	if err != nil {
		t.Fatalf("begin prepared pass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetVertexBuffer(0, vertex, 0)
	pass.SetIndexBuffer(index, gputypes.IndexFormatUint32, 0)
	if err := pass.ExecutePreparedIndexedCommands(prepared); err != nil {
		t.Fatalf("execute prepared indexed records: %v", err)
	}
	if err := pass.End(); err != nil {
		t.Fatalf("end prepared pass: %v", err)
	}
	return submitParityReadback(t, device, queue, encoder, target, "prepared")
}

func recordOracleTarget(t *testing.T, device *wgpu.Device, queue *wgpu.Queue, pipeline *wgpu.RenderPipeline, vertex, index, indirect *wgpu.Buffer) []byte {
	t.Helper()
	target, view := newParityTarget(t, device, "oracle")
	defer target.Release()
	defer view.Release()
	encoder, err := device.CreateCommandEncoder(nil)
	if err != nil {
		t.Fatalf("create oracle encoder: %v", err)
	}
	pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{ColorAttachments: []wgpu.RenderPassColorAttachment{{View: view, LoadOp: gputypes.LoadOpClear, StoreOp: gputypes.StoreOpStore}}})
	if err != nil {
		t.Fatalf("begin oracle pass: %v", err)
	}
	pass.SetPipeline(pipeline)
	pass.SetVertexBuffer(0, vertex, 0)
	pass.SetIndexBuffer(index, gputypes.IndexFormatUint32, 0)
	pass.DrawIndexedIndirect(indirect, 0)
	pass.DrawIndexedIndirect(indirect, 20)
	if err := pass.End(); err != nil {
		t.Fatalf("end oracle pass: %v", err)
	}
	return submitParityReadback(t, device, queue, encoder, target, "oracle")
}

func newParityTarget(t *testing.T, device *wgpu.Device, label string) (*wgpu.Texture, *wgpu.TextureView) {
	t.Helper()
	target, err := device.CreateTexture(&wgpu.TextureDescriptor{Label: label + " target", Size: wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1}, MipLevelCount: 1, SampleCount: 1, Dimension: wgpu.TextureDimension2D, Format: wgpu.TextureFormatRGBA8Unorm, Usage: wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc})
	if err != nil {
		t.Fatalf("create %s target: %v", label, err)
	}
	view, err := device.CreateTextureView(target, nil)
	if err != nil {
		target.Release()
		t.Fatalf("create %s target view: %v", label, err)
	}
	return target, view
}

func submitParityReadback(t *testing.T, device *wgpu.Device, queue *wgpu.Queue, encoder *wgpu.CommandEncoder, target *wgpu.Texture, label string) []byte {
	t.Helper()
	readback, err := device.CreateBuffer(&wgpu.BufferDescriptor{Label: label + " readback", Size: 4 * 256, Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead})
	if err != nil {
		t.Fatalf("create %s readback: %v", label, err)
	}
	defer readback.Release()
	encoder.CopyTextureToBuffer(target, readback, []wgpu.BufferTextureCopy{{TextureBase: wgpu.ImageCopyTexture{Texture: target}, BufferLayout: wgpu.ImageDataLayout{BytesPerRow: 256, RowsPerImage: 4}, Size: wgpu.Extent3D{Width: 4, Height: 4, DepthOrArrayLayers: 1}}})
	commandBuffer, err := encoder.Finish()
	if err != nil {
		t.Fatalf("finish %s encoder: %v", label, err)
	}
	if _, err := queue.Submit(commandBuffer); err != nil {
		t.Fatalf("submit %s command buffer: %v", label, err)
	}
	device.Poll(wgpu.PollWait)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := readback.Map(ctx, wgpu.MapModeRead, 0, 4*256); err != nil {
		t.Fatalf("map %s readback: %v", label, err)
	}
	rangeView, err := readback.MappedRange(0, 4*256)
	if err != nil {
		_ = readback.Unmap()
		t.Fatalf("read %s readback: %v", label, err)
	}
	pixels := append([]byte(nil), rangeView.Bytes()...)
	if err := readback.Unmap(); err != nil {
		t.Fatalf("unmap %s readback: %v", label, err)
	}
	return pixels
}
