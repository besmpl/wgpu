//go:build darwin && !(js && wasm)

package metal

import (
	"testing"

	"github.com/gogpu/wgpu/hal"
)

func TestPreparedIndexedRejectsLostEncoderGeneration(t *testing.T) {
	encoder := &CommandEncoder{cmdBuffer: 1, generation: 2}
	token := &preparedIndexedCommands{encoder: encoder, generation: 1}
	if err := (&RenderPassEncoder{}).ExecutePreparedIndexed(token); err != hal.ErrPreparedIndexedOwnership {
		t.Fatalf("stale token error = %v, want ownership", err)
	}
}
