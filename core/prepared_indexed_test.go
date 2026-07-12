//go:build !(js && wasm)

package core

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

type preparedMockToken struct{ batch hal.IndexedBatchDescriptor }

type preparedMockPass struct {
	noop.RenderPassEncoder
	executed int
	batch    hal.IndexedBatchDescriptor
}

func (p *preparedMockPass) ExecutePreparedIndexed(token hal.PreparedIndexedCommands) error {
	prepared, ok := token.(*preparedMockToken)
	if !ok {
		return errors.New("unexpected token")
	}
	p.batch = prepared.batch
	p.executed++
	return nil
}

type preparedMockEncoder struct {
	noop.CommandEncoder
	pass *preparedMockPass
}

func (e *preparedMockEncoder) PrepareIndexed(batch hal.IndexedBatchDescriptor) (hal.PreparedIndexedCommands, error) {
	return &preparedMockToken{batch: batch}, nil
}

func (e *preparedMockEncoder) BeginRenderPass(_ *hal.RenderPassDescriptor) hal.RenderPassEncoder {
	if e.pass == nil {
		e.pass = &preparedMockPass{}
	}
	return e.pass
}

type preparedMockDevice struct {
	noop.Device
	lastEncoder *preparedMockEncoder
}

func (d *preparedMockDevice) CreateCommandEncoder(_ *hal.CommandEncoderDescriptor) (hal.CommandEncoder, error) {
	encoder := &preparedMockEncoder{}
	d.lastEncoder = encoder
	return encoder, nil
}

func TestPreparedIndexedCoreLifecycleAndForwarding(t *testing.T) {
	halDevice := &preparedMockDevice{}
	device := NewDevice(halDevice, &Adapter{}, 0, gputypes.DefaultLimits(), "prepared-test")
	args, err := device.CreateBuffer(&gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndirect})
	if err != nil {
		t.Fatal(err)
	}
	index, err := device.CreateBuffer(&gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndex})
	if err != nil {
		t.Fatal(err)
	}
	enc, err := device.CreateCommandEncoder("prepared")
	if err != nil {
		t.Fatal(err)
	}
	token, err := enc.PrepareIndexedCommands(args, 0, 2, index, 0, gputypes.IndexFormatUint32)
	if err != nil {
		t.Fatal(err)
	}
	pass, err := enc.BeginRenderPass(&RenderPassDescriptor{})
	if err != nil {
		t.Fatal(err)
	}
	if err := pass.ExecutePreparedIndexedCommands(token); err != nil {
		t.Fatal(err)
	}
	guard := device.snatchLock.Read()
	wantArguments := args.Raw(guard)
	wantIndex := index.Raw(guard)
	guard.Release()
	if got := halDevice.lastEncoder.pass.batch; got.Arguments != wantArguments || got.IndexBuffer != wantIndex {
		t.Fatalf("forwarded resource identities changed")
	}
	if got := halDevice.lastEncoder.pass.batch.ArgumentsOffset; got != 0 {
		t.Fatalf("arguments offset = %d, want 0", got)
	}
	if got := halDevice.lastEncoder.pass.batch.Count; got != 2 {
		t.Fatalf("count = %d, want 2", got)
	}
	if got := halDevice.lastEncoder.pass.batch.IndexOffset; got != 0 {
		t.Fatalf("index offset = %d, want 0", got)
	}
	if got := halDevice.lastEncoder.pass.batch.IndexFormat; got != gputypes.IndexFormatUint32 {
		t.Fatalf("index format = %v, want uint32", got)
	}
	if err := pass.ExecutePreparedIndexedCommands(token); !errors.Is(err, hal.ErrPreparedIndexedState) {
		t.Fatalf("duplicate execute error = %v, want state error", err)
	}
	if err := pass.End(); err != nil {
		t.Fatal(err)
	}
	if _, err := enc.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedIndexedCoreRejectsOpenPassAndCrossEncoder(t *testing.T) {
	device := NewDevice(&preparedMockDevice{}, &Adapter{}, 0, gputypes.DefaultLimits(), "prepared-test")
	args, _ := device.CreateBuffer(&gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndirect})
	index, _ := device.CreateBuffer(&gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndex})
	enc, _ := device.CreateCommandEncoder("prepared")
	pass, _ := enc.BeginRenderPass(&RenderPassDescriptor{})
	if _, err := enc.PrepareIndexedCommands(args, 0, 1, index, 0, gputypes.IndexFormatUint32); err == nil {
		t.Fatal("preparation during an open pass unexpectedly succeeded")
	}
	_ = pass.End()
	other, _ := device.CreateCommandEncoder("other")
	token, err := enc.PrepareIndexedCommands(args, 0, 1, index, 0, gputypes.IndexFormatUint32)
	if err != nil {
		t.Fatal(err)
	}
	otherPass, _ := other.BeginRenderPass(&RenderPassDescriptor{})
	if err := otherPass.ExecutePreparedIndexedCommands(token); !errors.Is(err, hal.ErrPreparedIndexedUnsupported) && !errors.Is(err, hal.ErrPreparedIndexedOwnership) {
		t.Fatalf("cross-encoder execute error = %v", err)
	}
}

func TestPreparedIndexedCoreRejectsDestroyedBuffer(t *testing.T) {
	device := NewDevice(&preparedMockDevice{}, &Adapter{}, 0, gputypes.DefaultLimits(), "prepared-test")
	args, _ := device.CreateBuffer(&gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndirect})
	index, _ := device.CreateBuffer(&gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndex})
	args.Destroy()
	enc, _ := device.CreateCommandEncoder("prepared")
	if _, err := enc.PrepareIndexedCommands(args, 0, 1, index, 0, gputypes.IndexFormatUint32); !errors.Is(err, hal.ErrPreparedIndexedInvalid) {
		t.Fatalf("destroyed argument buffer error = %v, want invalid", err)
	}
}

func TestPreparedIndexedCoreRejectsMalformedDescriptors(t *testing.T) {
	tests := []struct {
		name        string
		args        *gputypes.BufferDescriptor
		index       *gputypes.BufferDescriptor
		argsOffset  uint64
		count       uint32
		indexOffset uint64
		format      gputypes.IndexFormat
		wantErr     error
	}{
		{name: "argument alignment", argsOffset: 2, count: 1, indexOffset: 0, format: gputypes.IndexFormatUint32},
		{name: "index alignment", argsOffset: 0, count: 1, indexOffset: 2, format: gputypes.IndexFormatUint32},
		{name: "argument range", args: &gputypes.BufferDescriptor{Size: 20, Usage: gputypes.BufferUsageIndirect}, count: 2, format: gputypes.IndexFormatUint32},
		{name: "missing indirect usage", args: &gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndex}, count: 1, format: gputypes.IndexFormatUint32},
		{name: "missing index usage", index: &gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndirect}, count: 1, format: gputypes.IndexFormatUint32},
		{name: "invalid format", count: 1, format: gputypes.IndexFormat(99)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			device := NewDevice(&preparedMockDevice{}, &Adapter{}, 0, gputypes.DefaultLimits(), "prepared-validation")
			argsDesc := tc.args
			if argsDesc == nil {
				argsDesc = &gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndirect}
			}
			indexDesc := tc.index
			if indexDesc == nil {
				indexDesc = &gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndex}
			}
			args, err := device.CreateBuffer(argsDesc)
			if err != nil {
				t.Fatal(err)
			}
			index, err := device.CreateBuffer(indexDesc)
			if err != nil {
				t.Fatal(err)
			}
			enc, err := device.CreateCommandEncoder("prepared-validation")
			if err != nil {
				t.Fatal(err)
			}
			_, err = enc.PrepareIndexedCommands(args, tc.argsOffset, tc.count, index, tc.indexOffset, tc.format)
			if !errors.Is(err, hal.ErrPreparedIndexedInvalid) {
				t.Fatalf("prepare error = %v, want invalid", err)
			}
		})
	}
}

func TestPreparedIndexedCoreRejectsDestroyedIndexAndCrossDevice(t *testing.T) {
	device := NewDevice(&preparedMockDevice{}, &Adapter{}, 0, gputypes.DefaultLimits(), "prepared-validation")
	args, _ := device.CreateBuffer(&gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndirect})
	index, _ := device.CreateBuffer(&gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndex})
	index.Destroy()
	enc, _ := device.CreateCommandEncoder("prepared-validation")
	if _, err := enc.PrepareIndexedCommands(args, 0, 1, index, 0, gputypes.IndexFormatUint32); !errors.Is(err, hal.ErrPreparedIndexedInvalid) {
		t.Fatalf("destroyed index error = %v, want invalid", err)
	}

	other := NewDevice(&preparedMockDevice{}, &Adapter{}, 0, gputypes.DefaultLimits(), "prepared-other")
	otherIndex, _ := other.CreateBuffer(&gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndex})
	if _, err := enc.PrepareIndexedCommands(args, 0, 1, otherIndex, 0, gputypes.IndexFormatUint32); !errors.Is(err, hal.ErrPreparedIndexedInvalid) {
		t.Fatalf("cross-device error = %v, want invalid", err)
	}
}

func TestPreparedIndexedCoreRejectsLostDeviceBeforeHAL(t *testing.T) {
	device := NewDevice(&preparedMockDevice{}, &Adapter{}, 0, gputypes.DefaultLimits(), "prepared-test")
	args, _ := device.CreateBuffer(&gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndirect})
	index, _ := device.CreateBuffer(&gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndex})
	enc, _ := device.CreateCommandEncoder("prepared")
	token, err := enc.PrepareIndexedCommands(args, 0, 1, index, 0, gputypes.IndexFormatUint32)
	if err != nil {
		t.Fatal(err)
	}
	pass, _ := enc.BeginRenderPass(&RenderPassDescriptor{})
	initialGeneration := device.Generation()
	device.Destroy()
	if got := device.Generation(); got != initialGeneration+1 {
		t.Fatalf("destroy generation = %d, want %d", got, initialGeneration+1)
	}
	device.Destroy()
	if got := device.Generation(); got != initialGeneration+1 {
		t.Fatalf("repeated destroy generation = %d, want %d", got, initialGeneration+1)
	}
	if err := pass.ExecutePreparedIndexedCommands(token); !errors.Is(err, hal.ErrPreparedIndexedOwnership) {
		t.Fatalf("lost device error = %v, want ownership", err)
	}
}

func TestPreparedIndexedCoreRejectsGenerationMismatch(t *testing.T) {
	device := NewDevice(&preparedMockDevice{}, &Adapter{}, 0, gputypes.DefaultLimits(), "prepared-generation")
	args, _ := device.CreateBuffer(&gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndirect})
	index, _ := device.CreateBuffer(&gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndex})
	enc, _ := device.CreateCommandEncoder("prepared-generation")
	token, err := enc.PrepareIndexedCommands(args, 0, 1, index, 0, gputypes.IndexFormatUint32)
	if err != nil {
		t.Fatal(err)
	}
	pass, _ := enc.BeginRenderPass(&RenderPassDescriptor{})
	token.generation++
	if err := pass.ExecutePreparedIndexedCommands(token); !errors.Is(err, hal.ErrPreparedIndexedOwnership) {
		t.Fatalf("generation mismatch error = %v, want ownership", err)
	}
}

func TestPreparedIndexedCoreTokenCannotOutliveFinish(t *testing.T) {
	device := NewDevice(&preparedMockDevice{}, &Adapter{}, 0, gputypes.DefaultLimits(), "prepared-finish")
	args, _ := device.CreateBuffer(&gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndirect})
	index, _ := device.CreateBuffer(&gputypes.BufferDescriptor{Size: 64, Usage: gputypes.BufferUsageIndex})
	enc, _ := device.CreateCommandEncoder("prepared-finish")
	if _, err := enc.PrepareIndexedCommands(args, 0, 1, index, 0, gputypes.IndexFormatUint32); err != nil {
		t.Fatal(err)
	}
	if _, err := enc.Finish(); err != nil {
		t.Fatal(err)
	}
	if _, err := enc.BeginRenderPass(&RenderPassDescriptor{}); err == nil {
		t.Fatal("begin render pass after finish unexpectedly succeeded")
	}
}
