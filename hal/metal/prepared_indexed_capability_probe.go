//go:build darwin && hearth_prepared_indexed_probe && !(js && wasm)

package metal

import (
	"fmt"

	"github.com/besmpl/wgpu/hal"
	"github.com/gogpu/gputypes"
)

// The probe build is intentionally separate from ordinary capability
// discovery. It checks the exact family and selectors needed by the route,
// then creates and tears down a real ICB plus translator pipeline. A complete
// visible two-record render/readback proof remains the promotion gate.
func preparedIndexedCapabilities(d *Device) hal.PreparedIndexedCapabilities {
	if d == nil || d.raw == 0 || !preparedIndexedProvenFamily(d.raw) {
		return hal.PreparedIndexedCapabilities{}
	}
	if err := preparedIndexedSelectorCheck(d.raw); err != nil {
		hal.Logger().Warn("metal: prepared indexed probe selectors unavailable", "error", err)
		return hal.PreparedIndexedCapabilities{}
	}
	if err := preparedIndexedICBTranslatorProbe(d); err != nil {
		hal.Logger().Warn("metal: prepared indexed ICB/translator probe failed", "error", err)
		return hal.PreparedIndexedCapabilities{}
	}
	return hal.PreparedIndexedCapabilities{PreparedIndexed: true, MaxCommands: preparedIndexedMaxCommandsForFamily(MTLGPUFamilyApple7)}
}

func preparedIndexedICBTranslatorProbe(d *Device) error {
	maxCommands := preparedIndexedMaxCommandsForFamily(MTLGPUFamilyApple7)
	if maxCommands == 0 {
		return fmt.Errorf("normalized prepared indexed limit is zero")
	}
	argsRaw, err := d.CreateBuffer(&hal.BufferDescriptor{Size: uint64(maxCommands) * preparedIndexedCommandBytes, Usage: gputypes.BufferUsageIndirect | gputypes.BufferUsageStorage})
	if err != nil {
		return fmt.Errorf("argument buffer: %w", err)
	}
	args := argsRaw.(*Buffer)
	defer args.Destroy()
	indexRaw, err := d.CreateBuffer(&hal.BufferDescriptor{Size: 6, Usage: gputypes.BufferUsageIndex | gputypes.BufferUsageCopyDst})
	if err != nil {
		return fmt.Errorf("index buffer: %w", err)
	}
	index := indexRaw.(*Buffer)
	defer index.Destroy()

	encoderRaw, err := d.CreateCommandEncoder(nil)
	if err != nil {
		return err
	}
	encoder := encoderRaw.(*CommandEncoder)
	if err := encoder.BeginEncoding("hearth-prepared-indexed-probe"); err != nil {
		return err
	}
	defer encoder.DiscardEncoding()
	// Prepare two legal sets on one encoder. They must share one arena while
	// retaining disjoint command ranges; this is the native lifecycle proof for
	// the amortization path (the renderer proof owns visible pixel parity).
	prepared, err := encoder.PrepareIndexed(hal.IndexedBatchDescriptor{
		Arguments: args, Count: 8, ArgumentsOffset: 0, IndexBuffer: index, IndexFormat: gputypes.IndexFormatUint16,
	})
	if err != nil {
		return fmt.Errorf("ICB/translator creation: %w", err)
	}
	first, ok := prepared.(*preparedIndexedCommands)
	if !ok || first == nil {
		return fmt.Errorf("unexpected first preparation token type %T", prepared)
	}
	secondPrepared, err := encoder.PrepareIndexed(hal.IndexedBatchDescriptor{
		Arguments: args, Count: 4, ArgumentsOffset: 8 * preparedIndexedCommandBytes, IndexBuffer: index, IndexFormat: gputypes.IndexFormatUint16,
	})
	if err != nil {
		return fmt.Errorf("second ICB/translator creation: %w", err)
	}
	second, ok := secondPrepared.(*preparedIndexedCommands)
	if !ok || second == nil {
		return fmt.Errorf("unexpected second preparation token type %T", secondPrepared)
	}
	if first.arena == nil || first.arena != second.arena || second.commandBase != first.commandBase+first.batch.Count {
		return fmt.Errorf("arena ranges were not shared/disjoint: first=%#v second=%#v", first, second)
	}
	if !first.evidence.validFor(first.batch.Count) || !second.evidence.validFor(second.batch.Count) {
		return fmt.Errorf("unexpected preparation evidence: first=%#v second=%#v", first.evidence, second.evidence)
	}
	// Exercise the normalized finite allocation cap itself. This is a separate
	// arena because the small-set arena intentionally remains bounded/geometric.
	capPrepared, err := encoder.PrepareIndexed(hal.IndexedBatchDescriptor{
		Arguments: args, Count: maxCommands, IndexBuffer: index, IndexFormat: gputypes.IndexFormatUint16,
	})
	if err != nil {
		return fmt.Errorf("finite-cap ICB creation: %w", err)
	}
	capToken, ok := capPrepared.(*preparedIndexedCommands)
	if !ok || capToken == nil || capToken.arena == first.arena || capToken.arena.capacity != maxCommands || capToken.batch.Count != maxCommands {
		return fmt.Errorf("finite-cap arena mismatch: token=%#v", capPrepared)
	}
	oldArena := first.arena
	encoder.DiscardEncoding()
	// A subsequent encoding gets fresh owner-retained state; no token or ICB
	// from the discarded encoding may leak into the next frame.
	nextRaw, err := d.CreateCommandEncoder(nil)
	if err != nil {
		return err
	}
	next := nextRaw.(*CommandEncoder)
	if err := next.BeginEncoding("hearth-prepared-indexed-probe-next"); err != nil {
		return err
	}
	nextPrepared, err := next.PrepareIndexed(hal.IndexedBatchDescriptor{
		Arguments: args, Count: 4, IndexBuffer: index, IndexFormat: gputypes.IndexFormatUint16,
	})
	if err != nil {
		next.DiscardEncoding()
		return fmt.Errorf("next encoding preparation: %w", err)
	}
	nextToken := nextPrepared.(*preparedIndexedCommands)
	if nextToken.arena == oldArena || nextToken.commandBase != 0 {
		next.DiscardEncoding()
		return fmt.Errorf("next encoding reused discarded arena: arena=%p base=%d", nextToken.arena, nextToken.commandBase)
	}
	next.DiscardEncoding()
	return nil
}
