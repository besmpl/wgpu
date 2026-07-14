//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"testing"

	"github.com/besmpl/wgpu/hal"
	"github.com/gogpu/gputypes"
)

func TestRenderPassEncoderMultiDrawIndirectRecordsFixedStride(t *testing.T) {
	enc := &CommandEncoder{}
	if err := enc.BeginEncoding("indirect"); err != nil {
		t.Fatal(err)
	}
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{ColorAttachments: []hal.RenderPassColorAttachment{}})
	pass.DrawIndirect(&Buffer{id: 7, size: 52}, 4, 3)

	if len(enc.commands) != 3 {
		t.Fatalf("commands = %d, want 3", len(enc.commands))
	}
	for i, want := range []uint64{4, 20, 36} {
		cmd, ok := enc.commands[i].(*DrawIndirectCommand)
		if !ok {
			t.Fatalf("command %d = %T, want DrawIndirectCommand", i, enc.commands[i])
		}
		if cmd.buffer.id != 7 || cmd.offset != want {
			t.Fatalf("command %d = buffer %d offset %d, want buffer 7 offset %d", i, cmd.buffer.id, cmd.offset, want)
		}
	}
}

func TestRenderPassEncoderMultiDrawIndexedIndirectRecordsFixedStride(t *testing.T) {
	enc := &CommandEncoder{}
	if err := enc.BeginEncoding("indexed-indirect"); err != nil {
		t.Fatal(err)
	}
	pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{ColorAttachments: []hal.RenderPassColorAttachment{}})
	pass.SetIndexBuffer(&Buffer{id: 9, size: 64}, gputypes.IndexFormatUint32, 0)
	pass.DrawIndexedIndirect(&Buffer{id: 8, size: 44}, 4, 2)

	if len(enc.commands) != 3 {
		t.Fatalf("commands = %d, want 3", len(enc.commands))
	}
	for i, want := range []uint64{4, 24} {
		cmd, ok := enc.commands[i+1].(*DrawIndexedIndirectCommand)
		if !ok {
			t.Fatalf("command %d = %T, want DrawIndexedIndirectCommand", i+1, enc.commands[i+1])
		}
		if cmd.buffer.id != 8 || cmd.offset != want || cmd.indexFormat != gputypes.IndexFormatUint32 {
			t.Fatalf("command %d = buffer %d offset %d format %v, want buffer 8 offset %d format Uint32", i+1, cmd.buffer.id, cmd.offset, cmd.indexFormat, want)
		}
	}
}

func TestRenderPassEncoderMultiDrawIndirectRejectsInvalidRanges(t *testing.T) {
	tests := []struct {
		name       string
		bufferSize uint64
		offset     uint64
		drawCount  uint32
	}{
		{name: "undersized", bufferSize: 47, offset: 16, drawCount: 2},
		{name: "offset overflow", bufferSize: 64, offset: ^uint64(0) - 3, drawCount: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			enc := &CommandEncoder{}
			if err := enc.BeginEncoding("invalid-indirect"); err != nil {
				t.Fatal(err)
			}
			pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{ColorAttachments: []hal.RenderPassColorAttachment{}})
			pass.DrawIndirect(&Buffer{id: 7, size: test.bufferSize}, test.offset, test.drawCount)
			if len(enc.commands) != 0 {
				t.Fatalf("commands = %d, want zero for invalid range", len(enc.commands))
			}
		})
	}
}

func TestRenderPassEncoderMultiDrawIndexedIndirectRejectsInvalidRanges(t *testing.T) {
	tests := []struct {
		name       string
		bufferSize uint64
		offset     uint64
		drawCount  uint32
	}{
		{name: "undersized", bufferSize: 43, offset: 4, drawCount: 2},
		{name: "offset overflow", bufferSize: 64, offset: ^uint64(0) - 3, drawCount: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			enc := &CommandEncoder{}
			if err := enc.BeginEncoding("invalid-indexed-indirect"); err != nil {
				t.Fatal(err)
			}
			pass := enc.BeginRenderPass(&hal.RenderPassDescriptor{ColorAttachments: []hal.RenderPassColorAttachment{}})
			pass.SetIndexBuffer(&Buffer{id: 9, size: 64}, gputypes.IndexFormatUint32, 0)
			pass.DrawIndexedIndirect(&Buffer{id: 8, size: test.bufferSize}, test.offset, test.drawCount)
			if len(enc.commands) != 1 {
				t.Fatalf("commands = %d, want only SetIndexBufferCommand", len(enc.commands))
			}
		})
	}
}
