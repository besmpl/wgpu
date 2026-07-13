//go:build darwin && !(js && wasm) && hearth_gpu_timing_probe

package metal

import (
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

const (
	defaultTimingProbeCapacity = 16
	maxTimingProbeCapacity     = 64
)

// timingProbeState is allocated only when a caller explicitly arms the
// tagged proof. Records are retained by submission index in a bounded FIFO;
// callbacks that arrive after eviction are intentionally ignored.
type timingProbeState struct {
	mu       sync.Mutex
	capacity int
	records  map[uint64]*timingProbeRecord
	order    []uint64
}

var timingProbeStates sync.Map // map[*Queue]*timingProbeState

func (q *Queue) EnableGPUTimingProbe(capacity int) bool {
	if q == nil || symNSConcreteGlobalBlock == 0 || sharedEventBlockDescriptor == nil {
		return false
	}
	if capacity <= 0 {
		capacity = defaultTimingProbeCapacity
	}
	if capacity > maxTimingProbeCapacity {
		capacity = maxTimingProbeCapacity
	}
	timingProbeStates.Store(q, &timingProbeState{
		capacity: capacity,
		records:  make(map[uint64]*timingProbeRecord, capacity),
		order:    make([]uint64, 0, capacity),
	})
	return true
}

func (q *Queue) DisableGPUTimingProbe() {
	if q != nil {
		timingProbeStates.Delete(q)
	}
}

func (q *Queue) GPUTimingProbeResult(submission uint64) (start, end float64, commandBuffers uint32, ready bool) {
	state, ok := timingProbeStateFor(q)
	if !ok {
		return 0, 0, 0, false
	}
	state.mu.Lock()
	record := state.records[submission]
	if record == nil || !record.ready() {
		state.mu.Unlock()
		return 0, 0, 0, false
	}
	start, end, commandBuffers, ready = record.start, record.end, record.expected, true
	state.mu.Unlock()
	return start, end, commandBuffers, ready
}

func timingProbeStateFor(q *Queue) (*timingProbeState, bool) {
	if q == nil {
		return nil, false
	}
	value, ok := timingProbeStates.Load(q)
	if !ok {
		return nil, false
	}
	state, ok := value.(*timingProbeState)
	return state, ok && state != nil
}

func timingProbeBeginSubmission(q *Queue, submission uint64, commandBuffers uint32) {
	state, ok := timingProbeStateFor(q)
	if !ok {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	for len(state.order) >= state.capacity {
		oldest := state.order[0]
		state.order = state.order[1:]
		delete(state.records, oldest)
	}
	state.records[submission] = &timingProbeRecord{expected: commandBuffers, valid: true}
	state.order = append(state.order, submission)
}

func timingProbeAttachCommandBuffer(q *Queue, commandBuffer ID, submission uint64) {
	state, ok := timingProbeStateFor(q)
	if !ok {
		return
	}
	if commandBuffer == 0 {
		timingProbeMarkFailure(state, submission)
		return
	}
	blockPtr := newTimingProbeCompletionBlock(state, submission)
	if blockPtr == 0 {
		timingProbeMarkFailure(state, submission)
		return
	}
	_ = MsgSend(commandBuffer, Sel("addCompletedHandler:"), blockPtr)
}

func timingProbeMarkFailure(state *timingProbeState, submission uint64) {
	state.mu.Lock()
	if record := state.records[submission]; record != nil {
		record.valid = false
		record.completed = record.expected
	}
	state.mu.Unlock()
}

type timingProbeCompletionEntry struct {
	state      *timingProbeState
	submission uint64
}

var (
	timingProbeCompletionRegistry sync.Map // map[uint64]*timingProbeCompletionEntry
	timingProbeCompletionOnce     sync.Once
	timingProbeCompletionInvoke   uintptr
)

func newTimingProbeCompletionBlock(state *timingProbeState, submission uint64) uintptr {
	if state == nil || symNSConcreteGlobalBlock == 0 || sharedEventBlockDescriptor == nil {
		return 0
	}
	invoke := timingProbeCompletionInvokePtr()
	if invoke == 0 {
		return 0
	}
	id := nextBlockID()
	timingProbeCompletionRegistry.Store(id, &timingProbeCompletionEntry{state: state, submission: submission})
	block := &blockLiteral{
		isa:        symNSConcreteGlobalBlock,
		flags:      blockIsGlobal,
		reserved:   0,
		invoke:     invoke,
		descriptor: uintptr(unsafe.Pointer(sharedEventBlockDescriptor)),
		blockID:    id,
	}
	blockPinRegistry.Store(id, block)
	return uintptr(unsafe.Pointer(block))
}

func timingProbeCompletionInvokePtr() uintptr {
	timingProbeCompletionOnce.Do(func() {
		// MTLCommandBuffer invokes completion handlers after the command buffer
		// reaches a terminal status. Read GPUStartTime/GPUEndTime only after
		// confirming Completed; PollWait is not used as a timing measurement.
		timingProbeCompletionInvoke = ffi.NewCallback(func(blockPtr, commandBuffer uintptr) uintptr {
			if blockPtr == 0 {
				return 0
			}
			id := *(*uint64)(unsafe.Pointer(blockPtr + 32)) //nolint:govet // ObjC block ABI
			blockPinRegistry.Delete(id)
			value, ok := timingProbeCompletionRegistry.LoadAndDelete(id)
			if !ok {
				return 0
			}
			entry := value.(*timingProbeCompletionEntry)
			status := MTLCommandBufferStatus(MsgSendUint(ID(commandBuffer), Sel("status")))
			completed := status == MTLCommandBufferStatusCompleted
			start, end := 0.0, 0.0
			if completed {
				start = timingProbeMsgSendDouble(ID(commandBuffer), Sel("GPUStartTime"))
				end = timingProbeMsgSendDouble(ID(commandBuffer), Sel("GPUEndTime"))
			}
			entry.state.mu.Lock()
			if record := entry.state.records[entry.submission]; record != nil {
				record.addSample(start, end, completed)
			}
			entry.state.mu.Unlock()
			return 0
		})
	})
	return timingProbeCompletionInvoke
}

func timingProbeMsgSendDouble(object ID, selector SEL) float64 {
	var result float64
	_ = msgSend(object, selector, types.DoubleTypeDescriptor, unsafe.Pointer(&result))
	return result
}
