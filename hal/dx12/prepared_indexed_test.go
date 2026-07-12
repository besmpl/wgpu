//go:build windows && !(js && wasm)

package dx12

import (
	"testing"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/dx12/d3d12"
)

func TestPreparedIndexedExecutesOneExecuteIndirectCall(t *testing.T) {
	encoder := &CommandEncoder{isRecording: true, device: &Device{}}
	pass := &RenderPassEncoder{encoder: encoder}
	buffer := &Buffer{}
	token := &preparedIndexedCommands{batch: hal.IndexedBatchDescriptor{Arguments: buffer, ArgumentsOffset: 40, Count: 3}}
	var gotCount uint32
	var gotOffset uint64
	old := preparedIndexedExecuteIndirect
	preparedIndexedExecuteIndirect = func(_ *d3d12.ID3D12GraphicsCommandList, _ *d3d12.ID3D12CommandSignature, count uint32, _ *d3d12.ID3D12Resource, offset uint64, _ *d3d12.ID3D12Resource, _ uint64) {
		gotCount, gotOffset = count, offset
	}
	defer func() { preparedIndexedExecuteIndirect = old }()
	if err := pass.ExecutePreparedIndexed(token); err != nil {
		t.Fatal(err)
	}
	if gotCount != 3 || gotOffset != 40 {
		t.Fatalf("ExecuteIndirect call = count %d offset %d, want 3/40", gotCount, gotOffset)
	}
}

func TestPreparedIndexedUsesBaselineExecuteIndirectCapability(t *testing.T) {
	d := &Device{}
	if d.PreparedIndexedRequiresFeature() {
		t.Fatal("baseline DX12 ExecuteIndirect route unexpectedly requires MultiDrawIndirect")
	}
	if caps := d.PreparedIndexedCapabilities(); !caps.PreparedIndexed || caps.MaxCommands == 0 {
		t.Fatalf("baseline DX12 capability = %+v, want supported finite limit", caps)
	}
}
