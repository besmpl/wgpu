//go:build !(js && wasm)

package core

import (
	"fmt"
	"sync/atomic"

	"github.com/besmpl/wgpu/hal"
	"github.com/gogpu/gputypes"
)

// PreparedIndexedCommands is an opaque one-shot token owned by its parent
// command encoder. Copies share the consumed bit and therefore cannot record
// the native operation twice.
type PreparedIndexedCommands struct {
	encoder    *CoreCommandEncoder
	device     *Device
	generation uint64
	prepared   hal.PreparedIndexedCommands
	arguments  *Buffer
	index      *Buffer
	offset     uint64
	count      uint32
	format     gputypes.IndexFormat
	consumed   atomic.Bool
}

// PrepareIndexedCommands prepares a native indexed multi-draw range while the
// command encoder is recording and no pass is open. Backends opt in through a
// narrow HAL interface; absence is reported before native recording begins.
func (e *CoreCommandEncoder) PrepareIndexedCommands(arguments *Buffer, argumentsOffset uint64, count uint32, index *Buffer, indexOffset uint64, format gputypes.IndexFormat) (*PreparedIndexedCommands, error) {
	if e == nil {
		return nil, fmt.Errorf("core: nil command encoder")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.Status() != CommandEncoderStatusRecording {
		return nil, fmt.Errorf("core: prepared indexed commands require recording encoder")
	}
	if e.mutable.activePass != nil {
		return nil, fmt.Errorf("core: prepared indexed commands cannot be prepared while a pass is open")
	}
	if arguments == nil || index == nil || count == 0 {
		return nil, hal.ErrPreparedIndexedInvalid
	}
	if arguments.IsDestroyed() || index.IsDestroyed() {
		return nil, hal.ErrPreparedIndexedInvalid
	}
	if arguments.Device() != e.device || index.Device() != e.device {
		return nil, fmt.Errorf("core: %w", hal.ErrPreparedIndexedInvalid)
	}
	if arguments.Usage()&gputypes.BufferUsageIndirect == 0 || index.Usage()&gputypes.BufferUsageIndex == 0 {
		return nil, hal.ErrPreparedIndexedInvalid
	}
	if format != gputypes.IndexFormatUint16 && format != gputypes.IndexFormatUint32 {
		return nil, hal.ErrPreparedIndexedInvalid
	}
	if argumentsOffset%4 != 0 || (format == gputypes.IndexFormatUint16 && indexOffset%2 != 0) || (format == gputypes.IndexFormatUint32 && indexOffset%4 != 0) {
		return nil, hal.ErrPreparedIndexedInvalid
	}
	indexBytes := uint64(2)
	if format == gputypes.IndexFormatUint32 {
		indexBytes = 4
	}
	if indexOffset > ^uint64(0)-indexBytes || indexOffset+indexBytes > index.Size() {
		return nil, hal.ErrPreparedIndexedInvalid
	}
	if uint64(count) > ^uint64(0)/20 || uint64(argumentsOffset)+uint64(count)*20 > arguments.Size() {
		return nil, hal.ErrPreparedIndexedInvalid
	}

	guard := e.device.snatchLock.Read()
	defer guard.Release()
	halEncoder := e.raw.Get(guard)
	if halEncoder == nil {
		return nil, fmt.Errorf("core: %w", hal.ErrPreparedIndexedUnsupported)
	}
	prep, ok := (*halEncoder).(hal.PreparedIndexedCommandEncoder)
	if !ok {
		return nil, hal.ErrPreparedIndexedUnsupported
	}
	argumentsRaw := arguments.Raw(guard)
	indexRaw := index.Raw(guard)
	if argumentsRaw == nil || indexRaw == nil {
		return nil, fmt.Errorf("core: %w", hal.ErrPreparedIndexedInvalid)
	}
	prepared, err := prep.PrepareIndexed(hal.IndexedBatchDescriptor{
		Arguments: argumentsRaw, ArgumentsOffset: uint64(argumentsOffset), Count: count,
		IndexBuffer: indexRaw, IndexOffset: indexOffset, IndexFormat: format,
	})
	if err != nil {
		return nil, err
	}
	if prepared == nil {
		return nil, fmt.Errorf("core: %w", hal.ErrPreparedIndexedUnsupported)
	}
	generation := e.device.Generation()

	if e.mutable.usedBuffers == nil {
		e.mutable.usedBuffers = make(map[*Buffer]BufferUses)
	}
	e.mutable.usedBuffers[arguments] |= BufferUsesIndirect
	e.mutable.usedBuffers[index] |= BufferUsesIndex
	return &PreparedIndexedCommands{
		encoder: e, device: e.device, generation: generation, prepared: prepared,
		arguments: arguments, index: index, offset: indexOffset,
		count: count, format: format,
	}, nil
}

// ExecutePreparedIndexedCommands executes a token on the render pass that was
// created by the same command encoder. The token is consumed exactly once.
func (p *CoreRenderPassEncoder) ExecutePreparedIndexedCommands(token *PreparedIndexedCommands) error {
	if p == nil || p.ended {
		return fmt.Errorf("core: %w", hal.ErrPreparedIndexedState)
	}
	if token == nil || token.encoder == nil || token.device == nil {
		return fmt.Errorf("core: %w", hal.ErrPreparedIndexedInvalid)
	}
	if token.encoder != p.encoder || token.device != p.device {
		return fmt.Errorf("core: %w", hal.ErrPreparedIndexedOwnership)
	}
	if err := token.device.checkValid(); err != nil {
		return fmt.Errorf("core: %w: %v", hal.ErrPreparedIndexedOwnership, err)
	}
	if token.device.Generation() != token.generation {
		return fmt.Errorf("core: %w: device generation changed", hal.ErrPreparedIndexedOwnership)
	}
	if p.encoder.Status() != CommandEncoderStatusLocked {
		return fmt.Errorf("core: %w", hal.ErrPreparedIndexedState)
	}
	guard := p.device.snatchLock.Read()
	defer guard.Release()
	halEncoder := p.encoder.raw.Get(guard)
	if halEncoder == nil {
		return fmt.Errorf("core: %w", hal.ErrPreparedIndexedOwnership)
	}
	// The pass owns its HAL object; only the optional interface can consume it.
	exec, ok := p.raw.(hal.PreparedIndexedRenderPass)
	if !ok {
		return hal.ErrPreparedIndexedUnsupported
	}
	if !token.consumed.CompareAndSwap(false, true) {
		return hal.ErrPreparedIndexedState
	}
	if err := exec.ExecutePreparedIndexed(token.prepared); err != nil {
		return fmt.Errorf("core: %w: %v", hal.ErrPreparedIndexedBackend, err)
	}
	return nil
}

func (t *PreparedIndexedCommands) Arguments() *Buffer {
	if t == nil {
		return nil
	}
	return t.arguments
}
func (t *PreparedIndexedCommands) IndexBuffer() *Buffer {
	if t == nil {
		return nil
	}
	return t.index
}
func (t *PreparedIndexedCommands) IndexOffset() uint64 {
	if t == nil {
		return 0
	}
	return t.offset
}
func (t *PreparedIndexedCommands) IndexFormat() gputypes.IndexFormat {
	if t == nil {
		return 0
	}
	return t.format
}
func (t *PreparedIndexedCommands) Count() uint32 {
	if t == nil {
		return 0
	}
	return t.count
}
