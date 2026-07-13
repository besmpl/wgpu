//go:build darwin && !(js && wasm)

package metal

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/besmpl/wgpu/hal"
	"github.com/gogpu/gputypes"
)

type preparedIndexedEvidence struct {
	resetCalls     uint32
	dispatchCalls  uint32
	dispatchGroups uint32
	executeCalls   uint32
	executeRange   NSRange
}

func (e preparedIndexedEvidence) validFor(count uint32) bool {
	return e.resetCalls == 1 && e.dispatchCalls == 1 && e.dispatchGroups == preparedIndexedDispatchGroups(count)
}

func (e preparedIndexedEvidence) validExecution(count uint32) bool {
	return e.validExecutionAt(0, count)
}

func (e preparedIndexedEvidence) validExecutionAt(base, count uint32) bool {
	return e.executeCalls == 1 && e.executeRange.Location == NSUInteger(base) && e.executeRange.Length == NSUInteger(count)
}

type preparedIndexedTranslatorKey struct {
	format gputypes.IndexFormat
}

type preparedIndexedTranslator struct {
	library  ID
	pipeline ID
	owned    bool
}

type preparedIndexedParams struct {
	commandBase uint32
	count       uint32
}

// releaseOwned releases a translator compiled for one token when it was not
// admitted to the device cache. Cached translators are device-owned and are
// released by releasePreparedIndexedTranslators instead.
func (t *preparedIndexedTranslator) releaseOwned() {
	if t == nil || !t.owned {
		return
	}
	if t.pipeline != 0 {
		Release(t.pipeline)
	}
	if t.library != 0 {
		Release(t.library)
	}
	*t = preparedIndexedTranslator{}
}

// preparedIndexedCommands owns every Metal object created for one encoding.
// Ownership transfers from CommandEncoder to CommandBuffer at EndEncoding and
// is released by Destroy after queue completion (or immediately on discard).
type preparedIndexedCommands struct {
	encoder     *CommandEncoder
	generation  uint64
	batch       hal.IndexedBatchDescriptor
	arena       *preparedIndexedArena
	commandBase uint32
	arguments   ID
	index       ID
	evidence    preparedIndexedEvidence
	executed    bool
}

func (p *preparedIndexedCommands) release() {
	if p == nil {
		return
	}
	if p.arguments != 0 {
		Release(p.arguments)
		p.arguments = 0
	}
	if p.index != 0 {
		Release(p.index)
		p.index = 0
	}
	p.evidence = preparedIndexedEvidence{}
	p.encoder = nil
	p.arena = nil
}

// preparedIndexedArena amortizes the expensive Metal objects needed by many
// small homogeneous sets. Tokens retain their source buffers, but share an
// encoder-owned ICB and argument buffer through disjoint command ranges.
// Arenas are transferred to the command buffer at EndEncoding and released
// only after completion (or immediately on discard).
type preparedIndexedArena struct {
	buffer      ID
	argBuffer   ID
	capacity    uint32
	next        uint32
	translators map[gputypes.IndexFormat]preparedIndexedTranslator
}

func (a *preparedIndexedArena) reserve(count uint32) (uint32, bool) {
	if a == nil || count == 0 || a.next > a.capacity || count > a.capacity-a.next {
		return 0, false
	}
	base := a.next
	a.next += count
	return base, true
}

func (a *preparedIndexedArena) release() {
	if a == nil {
		return
	}
	if a.buffer != 0 {
		Release(a.buffer)
		a.buffer = 0
	}
	if a.argBuffer != 0 {
		Release(a.argBuffer)
		a.argBuffer = 0
	}
	for format, translator := range a.translators {
		translator.releaseOwned()
		delete(a.translators, format)
	}
}

const preparedIndexedArenaMinCommands uint32 = 256

func preparedIndexedArenaCapacity(count, max uint32) uint32 {
	if count == 0 || max == 0 {
		return 0
	}
	capacity := preparedIndexedArenaMinCommands
	if capacity > max {
		capacity = max
	}
	for capacity < count && capacity < max {
		next := capacity * 2
		if next < capacity || next > max {
			capacity = max
			break
		}
		capacity = next
	}
	if capacity < count {
		return 0
	}
	return capacity
}

var preparedIndexedTranslatorMu sync.Mutex

const preparedIndexedTranslatorCacheLimit = 64

func preparedIndexedTranslatorShouldCache(entries int) bool {
	return entries < preparedIndexedTranslatorCacheLimit
}

func (d *Device) preparedIndexedTranslator(format gputypes.IndexFormat, indexOffset uint64, count uint32) (preparedIndexedTranslator, error) {
	// Index offset and command count are dispatch parameters, not pipeline
	// specialization constants. Keeping the cache keyed by format means a
	// frame with many small sets reuses one translator per index type.
	key := preparedIndexedTranslatorKey{format: format}
	preparedIndexedTranslatorMu.Lock()
	defer preparedIndexedTranslatorMu.Unlock()
	if d.preparedIndexedTranslators == nil {
		d.preparedIndexedTranslators = make(map[preparedIndexedTranslatorKey]preparedIndexedTranslator)
	}
	if translator, ok := d.preparedIndexedTranslators[key]; ok {
		return translator, nil
	}
	library, pipeline, err := compilePreparedTranslator(d, format)
	if err != nil {
		return preparedIndexedTranslator{}, err
	}
	if !preparedIndexedTranslatorShouldCache(len(d.preparedIndexedTranslators)) {
		return preparedIndexedTranslator{library: library, pipeline: pipeline, owned: true}, nil
	}
	translator := preparedIndexedTranslator{library: library, pipeline: pipeline}
	d.preparedIndexedTranslators[key] = translator
	return translator, nil
}

func (d *Device) releasePreparedIndexedTranslators() {
	preparedIndexedTranslatorMu.Lock()
	defer preparedIndexedTranslatorMu.Unlock()
	for key, translator := range d.preparedIndexedTranslators {
		Release(translator.pipeline)
		Release(translator.library)
		delete(d.preparedIndexedTranslators, key)
	}
}

func (e *CommandEncoder) PrepareIndexed(batch hal.IndexedBatchDescriptor) (hal.PreparedIndexedCommands, error) {
	if e == nil || !e.IsRecording() {
		return nil, hal.ErrPreparedIndexedState
	}
	args, ok := batch.Arguments.(*Buffer)
	if !ok || args == nil || args.raw == 0 || args.usage&gputypes.BufferUsageIndirect == 0 || args.usage&gputypes.BufferUsageStorage == 0 {
		return nil, hal.ErrPreparedIndexedInvalid
	}
	index, ok := batch.IndexBuffer.(*Buffer)
	if !ok || index == nil || index.raw == 0 || index.usage&gputypes.BufferUsageIndex == 0 {
		return nil, hal.ErrPreparedIndexedInvalid
	}
	if batch.Count == 0 {
		return nil, hal.ErrPreparedIndexedInvalid
	}
	maxCommands := preparedIndexedMaxCommandsForFamily(MTLGPUFamilyApple7)
	if maxCommands == 0 || batch.Count > maxCommands {
		return nil, hal.ErrPreparedIndexedLimit
	}
	if batch.IndexFormat != gputypes.IndexFormatUint16 && batch.IndexFormat != gputypes.IndexFormatUint32 {
		return nil, hal.ErrPreparedIndexedInvalid
	}
	indexBytes := uint64(2)
	if batch.IndexFormat == gputypes.IndexFormatUint32 {
		indexBytes = 4
	}
	if batch.IndexOffset%indexBytes != 0 || batch.IndexOffset > ^uint64(0)-indexBytes || batch.IndexOffset+indexBytes > index.size {
		return nil, hal.ErrPreparedIndexedInvalid
	}
	if batch.ArgumentsOffset > ^uint64(0)-uint64(batch.Count)*20 || batch.ArgumentsOffset+uint64(batch.Count)*20 > args.size {
		return nil, hal.ErrPreparedIndexedInvalid
	}
	Retain(args.raw)
	Retain(index.raw)
	retainSources := true
	defer func() {
		if retainSources {
			Release(args.raw)
			Release(index.raw)
		}
	}()

	arena, err := e.acquirePreparedIndexedArena(batch.Count)
	if err != nil {
		return nil, err
	}
	commandBase, ok := arena.reserve(batch.Count)
	if !ok {
		return nil, hal.ErrPreparedIndexedLimit
	}
	rng := NSRange{Location: NSUInteger(commandBase), Length: NSUInteger(batch.Count)}
	if err := preparedIndexedObjectSelectorCheck(arena.buffer, "indirect command buffer", "resetWithRange:"); err != nil {
		return nil, err
	}
	msgSendVoid(arena.buffer, Sel("resetWithRange:"), argStruct(rng, nsRangeType))

	translator, err := e.preparedIndexedArenaTranslator(arena, batch.IndexFormat)
	if err != nil {
		return nil, err
	}

	compute := MsgSend(e.cmdBuffer, Sel("computeCommandEncoder"))
	if compute == 0 {
		return nil, fmt.Errorf("metal: failed to create indexed preparation compute encoder")
	}
	Retain(compute)
	_ = MsgSend(compute, Sel("setComputePipelineState:"), uintptr(translator.pipeline))
	_ = MsgSend(compute, Sel("setBuffer:offset:atIndex:"), uintptr(args.raw), uintptr(batch.ArgumentsOffset), uintptr(0))
	_ = MsgSend(compute, Sel("setBuffer:offset:atIndex:"), uintptr(arena.argBuffer), 0, uintptr(1))
	_ = MsgSend(compute, Sel("setBuffer:offset:atIndex:"), uintptr(index.raw), uintptr(batch.IndexOffset), uintptr(2))
	params := preparedIndexedParams{commandBase: commandBase, count: batch.Count}
	_ = MsgSend(compute, Sel("setBytes:length:atIndex:"), uintptr(unsafe.Pointer(&params)), uintptr(unsafe.Sizeof(params)), uintptr(3))
	_ = MsgSend(compute, Sel("useResource:usage:"), uintptr(arena.buffer), uintptr(2))
	groups := preparedIndexedDispatchGroups(batch.Count)
	msgSendVoid(compute, Sel("dispatchThreadgroups:threadsPerThreadgroup:"),
		argStruct(MTLSize{Width: NSUInteger(groups), Height: 1, Depth: 1}, mtlSizeType),
		argStruct(MTLSize{Width: 64, Height: 1, Depth: 1}, mtlSizeType))
	_ = MsgSend(compute, Sel("endEncoding"))
	Release(compute)

	prepared := &preparedIndexedCommands{encoder: e, generation: e.generation, batch: batch, arena: arena, commandBase: commandBase, arguments: args.raw, index: index.raw, evidence: preparedIndexedEvidence{resetCalls: 1, dispatchCalls: 1, dispatchGroups: groups}}
	e.prepared = append(e.prepared, prepared)
	retainSources = false
	return prepared, nil
}

func (e *CommandEncoder) acquirePreparedIndexedArena(count uint32) (*preparedIndexedArena, error) {
	maxCommands := preparedIndexedMaxCommandsForFamily(MTLGPUFamilyApple7)
	if len(e.preparedArenas) > 0 {
		arena := e.preparedArenas[len(e.preparedArenas)-1]
		if _, ok := arena.reserve(count); ok {
			arena.next -= count
			return arena, nil
		}
	}
	capacity := preparedIndexedArenaCapacity(count, maxCommands)
	if capacity == 0 {
		return nil, hal.ErrPreparedIndexedLimit
	}
	desc := MsgSend(ID(GetClass("MTLIndirectCommandBufferDescriptor")), Sel("alloc"))
	if desc == 0 {
		return nil, fmt.Errorf("metal: failed to allocate indirect command buffer descriptor")
	}
	desc = MsgSend(desc, Sel("init"))
	if desc == 0 {
		return nil, fmt.Errorf("metal: failed to initialize indirect command buffer descriptor")
	}
	defer Release(desc)
	if err := preparedIndexedObjectSelectorCheck(desc, "indirect command buffer descriptor", "setCommandTypes:", "setInheritPipelineState:", "setInheritBuffers:"); err != nil {
		return nil, err
	}
	_ = MsgSend(desc, Sel("setCommandTypes:"), uintptr(MTLIndirectCommandTypeDrawIndexed))
	_ = MsgSend(desc, Sel("setInheritPipelineState:"), uintptr(YES))
	_ = MsgSend(desc, Sel("setInheritBuffers:"), uintptr(YES))
	icb := MsgSend(e.device.raw, Sel("newIndirectCommandBufferWithDescriptor:maxCommandCount:options:"), uintptr(desc), uintptr(capacity), uintptr(0))
	if icb == 0 {
		return nil, fmt.Errorf("metal: failed to create indirect command buffer")
	}
	translator, argBuffer, err := e.newPreparedArenaArgumentBuffer(icb)
	if err != nil {
		Release(icb)
		return nil, err
	}
	arena := &preparedIndexedArena{buffer: icb, argBuffer: argBuffer, capacity: capacity, translators: make(map[gputypes.IndexFormat]preparedIndexedTranslator)}
	arena.translators[gputypes.IndexFormatUint32] = translator
	e.preparedArenas = append(e.preparedArenas, arena)
	return arena, nil
}

func (e *CommandEncoder) preparedIndexedArenaTranslator(arena *preparedIndexedArena, format gputypes.IndexFormat) (preparedIndexedTranslator, error) {
	if translator, ok := arena.translators[format]; ok {
		return translator, nil
	}
	translator, err := e.device.preparedIndexedTranslator(format, 0, 0)
	if err != nil {
		return preparedIndexedTranslator{}, err
	}
	arena.translators[format] = translator
	return translator, nil
}

func (e *CommandEncoder) newPreparedArenaArgumentBuffer(icb ID) (preparedIndexedTranslator, ID, error) {
	// The argument encoder stores only the shared arena ICB pointer. The
	// translator pipeline is selected per index format at dispatch time.
	translator, err := e.device.preparedIndexedTranslator(gputypes.IndexFormatUint32, 0, 0)
	if err != nil {
		return preparedIndexedTranslator{}, 0, err
	}
	fnName := NSString("hearth_prepare_indexed")
	fn := MsgSend(translator.library, Sel("newFunctionWithName:"), uintptr(fnName))
	Release(fnName)
	if fn == 0 {
		translator.releaseOwned()
		return preparedIndexedTranslator{}, 0, fmt.Errorf("metal: indexed translator function missing")
	}
	defer Release(fn)
	argEncoder := MsgSend(fn, Sel("newArgumentEncoderWithBufferIndex:"), uintptr(1))
	if argEncoder == 0 {
		translator.releaseOwned()
		return preparedIndexedTranslator{}, 0, fmt.Errorf("metal: indexed translator argument encoder creation failed")
	}
	defer Release(argEncoder)
	encodedLength := MsgSendUint(argEncoder, Sel("encodedLength"))
	argBuffer := MsgSend(e.device.raw, Sel("newBufferWithLength:options:"), uintptr(encodedLength), uintptr(MTLResourceStorageModePrivate))
	if argBuffer == 0 {
		translator.releaseOwned()
		return preparedIndexedTranslator{}, 0, fmt.Errorf("metal: indexed translator argument buffer creation failed")
	}
	_ = MsgSend(argEncoder, Sel("setArgumentBuffer:offset:"), uintptr(argBuffer), 0)
	_ = MsgSend(argEncoder, Sel("setIndirectCommandBuffer:atIndex:"), uintptr(icb), uintptr(0))
	return translator, argBuffer, nil
}

func compilePreparedTranslator(d *Device, format gputypes.IndexFormat) (ID, ID, error) {
	indexType := "uint"
	if format == gputypes.IndexFormatUint16 {
		indexType = "ushort"
	}
	source := fmt.Sprintf(`
#include <metal_stdlib>
#include <metal_command_buffer>
using namespace metal;
struct DrawIndexedArgs { uint indexCount; uint instanceCount; uint firstIndex; int baseVertex; uint firstInstance; };
struct ICBContainer { command_buffer commandBuffer [[id(0)]]; };
struct PreparedIndexedParams { uint commandBase; uint count; };
kernel void hearth_prepare_indexed(device const DrawIndexedArgs* args [[buffer(0)]],
    device ICBContainer* container [[buffer(1)]], device const %s* indices [[buffer(2)]],
    constant PreparedIndexedParams& params [[buffer(3)]],
    uint id [[thread_position_in_grid]]) {
    if (id >= params.count) return;
    DrawIndexedArgs a = args[id];
    render_command command(container->commandBuffer, params.commandBase + id);
    command.draw_indexed_primitives(primitive_type::triangle, a.indexCount,
        indices + a.firstIndex, a.instanceCount, a.baseVertex, a.firstInstance);
}
`, indexType)
	str := NSString(source)
	defer Release(str)
	var errorPtr ID
	library := MsgSend(d.raw, Sel("newLibraryWithSource:options:error:"), uintptr(str), 0, uintptr(unsafe.Pointer(&errorPtr)))
	if library == 0 {
		return 0, 0, fmt.Errorf("metal: indexed translator compilation failed: %s", formatNSError(errorPtr))
	}
	fnName := NSString("hearth_prepare_indexed")
	fn := MsgSend(library, Sel("newFunctionWithName:"), uintptr(fnName))
	Release(fnName)
	if fn == 0 {
		Release(library)
		return 0, 0, fmt.Errorf("metal: indexed translator function missing")
	}
	pipeline := MsgSend(d.raw, Sel("newComputePipelineStateWithFunction:error:"), uintptr(fn), uintptr(unsafe.Pointer(&errorPtr)))
	Release(fn)
	if pipeline == 0 {
		Release(library)
		return 0, 0, fmt.Errorf("metal: indexed translator pipeline creation failed: %s", formatNSError(errorPtr))
	}
	return library, pipeline, nil
}

func (e *RenderPassEncoder) ExecutePreparedIndexed(prepared hal.PreparedIndexedCommands) error {
	token, ok := prepared.(*preparedIndexedCommands)
	if !ok || token == nil || token.encoder == nil || token.arena == nil || token.encoder.cmdBuffer == 0 || token.generation != token.encoder.generation {
		return hal.ErrPreparedIndexedOwnership
	}
	if token.executed {
		return hal.ErrPreparedIndexedState
	}
	if e == nil || e.raw == 0 || e.indexBuffer == nil || e.indexBuffer != token.batch.IndexBuffer || e.indexFormat != token.batch.IndexFormat || e.indexOffset != token.batch.IndexOffset {
		return hal.ErrPreparedIndexedState
	}
	// ICB commands reference resources through inherited render state; declare
	// the index buffer explicitly so Metal's hazard tracker sees that read.
	_ = MsgSend(e.raw, Sel("useResource:usage:"), uintptr(e.indexBuffer.raw), uintptr(1))
	rng := NSRange{Location: NSUInteger(token.commandBase), Length: NSUInteger(token.batch.Count)}
	msgSendVoid(e.raw, Sel("executeCommandsInBuffer:withRange:"), argPointer(uintptr(token.arena.buffer)), argStruct(rng, nsRangeType))
	token.evidence.executeCalls++
	token.evidence.executeRange = rng
	token.executed = true
	return nil
}

func (d *Device) PreparedIndexedCapabilities() hal.PreparedIndexedCapabilities {
	return preparedIndexedCapabilities(d)
}

func (*Device) PreparedIndexedRequiresFeature() bool { return false }

var _ hal.PreparedIndexedCommandEncoder = (*CommandEncoder)(nil)
var _ hal.PreparedIndexedRenderPass = (*RenderPassEncoder)(nil)
var _ hal.PreparedIndexedCapabilityProvider = (*Device)(nil)
