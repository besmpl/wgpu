package wgpu

import (
	"errors"

	"github.com/gogpu/gputypes"
)

// PreparedIndexedCapabilities is the normalized capability exposed by a
// created device. MaxCommands is zero when the route is unsupported.
type PreparedIndexedCapabilities struct {
	PreparedIndexed bool
	MaxCommands     uint32
}

// PreparedIndexedCommandsDescriptor describes one homogeneous range of
// tightly packed 20-byte DrawIndexedIndirect records.
type PreparedIndexedCommandsDescriptor struct {
	Arguments       *Buffer
	ArgumentsOffset uint64
	Count           uint32
	IndexBuffer     *Buffer
	IndexOffset     uint64
	IndexFormat     IndexFormat
}

// PreparedIndexedCommands is an opaque, encoder-owned one-shot token. Its
// private state is shared by value copies, so copying a token cannot execute
// the prepared range twice.
type PreparedIndexedCommands struct{ state any }

var (
	ErrPreparedIndexedUnsupported     = errors.New("wgpu: prepared indexed commands are unsupported")
	ErrPreparedIndexedLimit           = errors.New("wgpu: prepared indexed command count exceeds the device limit")
	ErrPreparedIndexedInvalid         = errors.New("wgpu: prepared indexed command batch is invalid")
	ErrPreparedIndexedOwnership       = errors.New("wgpu: prepared indexed command belongs to another encoder or device")
	ErrPreparedIndexedAlreadyExecuted = errors.New("wgpu: prepared indexed command was already executed")
	ErrPreparedIndexedState           = errors.New("wgpu: prepared indexed command is in an invalid state")
	ErrPreparedIndexedBackend         = errors.New("wgpu: prepared indexed backend operation failed")
)

const preparedIndexedRecordSize uint64 = 20

func validatePreparedIndexedDescriptor(desc PreparedIndexedCommandsDescriptor, maxCommands uint32) error {
	if desc.Arguments == nil || desc.IndexBuffer == nil || desc.Count == 0 {
		return ErrPreparedIndexedInvalid
	}
	if desc.ArgumentsOffset%4 != 0 {
		return ErrPreparedIndexedInvalid
	}
	if desc.IndexFormat != gputypes.IndexFormatUint16 && desc.IndexFormat != gputypes.IndexFormatUint32 {
		return ErrPreparedIndexedInvalid
	}
	if desc.IndexFormat == gputypes.IndexFormatUint16 && desc.IndexOffset%2 != 0 {
		return ErrPreparedIndexedInvalid
	}
	if desc.IndexFormat == gputypes.IndexFormatUint32 && desc.IndexOffset%4 != 0 {
		return ErrPreparedIndexedInvalid
	}
	indexBytes := uint64(2)
	if desc.IndexFormat == gputypes.IndexFormatUint32 {
		indexBytes = 4
	}
	if desc.IndexOffset > ^uint64(0)-indexBytes || desc.IndexOffset+indexBytes > desc.IndexBuffer.Size() {
		return ErrPreparedIndexedInvalid
	}
	if desc.Count > maxCommands && maxCommands != 0 {
		return ErrPreparedIndexedLimit
	}
	if uint64(desc.Count) > ^uint64(0)/preparedIndexedRecordSize {
		return ErrPreparedIndexedInvalid
	}
	bytes := uint64(desc.Count) * preparedIndexedRecordSize
	if desc.ArgumentsOffset > ^uint64(0)-bytes || desc.ArgumentsOffset+bytes > desc.Arguments.Size() {
		return ErrPreparedIndexedInvalid
	}
	if desc.Arguments.Usage()&BufferUsageIndirect == 0 {
		return ErrPreparedIndexedInvalid
	}
	if desc.IndexBuffer.Usage()&BufferUsageIndex == 0 {
		return ErrPreparedIndexedInvalid
	}
	return nil
}
