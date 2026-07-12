//go:build !(js && wasm)

package vulkan

import (
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func TestPreparedIndexedForwardingCapturesFixedRange(t *testing.T) {
	encoder := &CommandEncoder{active: 1}
	args := &Buffer{}
	index := &Buffer{}
	token, err := encoder.PrepareIndexed(hal.IndexedBatchDescriptor{
		Arguments: args, ArgumentsOffset: 40, Count: 3,
		IndexBuffer: index, IndexOffset: 8, IndexFormat: gputypes.IndexFormatUint32,
	})
	if err != nil {
		t.Fatal(err)
	}
	prepared, ok := token.(*preparedIndexedCommands)
	if !ok || prepared.batch.ArgumentsOffset != 40 || prepared.batch.Count != 3 || prepared.batch.IndexOffset != 8 {
		t.Fatalf("prepared token = %#v, want fixed range metadata", token)
	}
}

func TestPreparedIndexedExecutesOneNativeCallWithRecordStride(t *testing.T) {
	encoder := &CommandEncoder{active: 1, device: &Device{}}
	pass := &RenderPassEncoder{encoder: encoder}
	args := &Buffer{handle: 7}
	token := &preparedIndexedCommands{batch: hal.IndexedBatchDescriptor{Arguments: args, ArgumentsOffset: 40, Count: 3}}
	var gotOffset uint64
	var gotCount, gotStride uint32
	old := preparedIndexedDrawIndexedIndirect
	preparedIndexedDrawIndexedIndirect = func(_ *vk.Commands, _ vk.CommandBuffer, _ vk.Buffer, offset vk.DeviceSize, count, stride uint32) {
		gotOffset, gotCount, gotStride = uint64(offset), count, stride
	}
	defer func() { preparedIndexedDrawIndexedIndirect = old }()
	if err := pass.ExecutePreparedIndexed(token); err != nil {
		t.Fatal(err)
	}
	if gotOffset != 40 || gotCount != 3 || gotStride != 20 {
		t.Fatalf("native call = offset %d count %d stride %d, want 40/3/20", gotOffset, gotCount, gotStride)
	}
}

func TestPreparedIndexedCapabilitiesUsePhysicalDeviceLimit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		device *Device
		want   hal.PreparedIndexedCapabilities
	}{
		{name: "nil device"},
		{name: "zero unsupported", device: &Device{}},
		{name: "physical limit", device: &Device{maxDrawIndirectCount: 17}, want: hal.PreparedIndexedCapabilities{PreparedIndexed: true, MaxCommands: 17}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.device.PreparedIndexedCapabilities(); got != tc.want {
				t.Fatalf("capabilities = %+v, want %+v", got, tc.want)
			}
		})
	}
}
