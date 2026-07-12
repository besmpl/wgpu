//go:build darwin && !(js && wasm)

package metal

const (
	preparedIndexedCommandBytes         uint64 = 20
	preparedIndexedProvenFamilyCap      uint32 = 65535
	preparedIndexedSafeAllocationBudget uint64 = 1 << 20
)

// normalizePreparedIndexedMaxCommands turns independent backend limits into
// one finite route limit. Keeping this pure makes the safety policy testable
// without an Objective-C device and prevents a raw hardware constant from
// becoming an unbounded allocation request.
func normalizePreparedIndexedMaxCommands(familyCap uint32, allocationBudget, commandBytes uint64) uint32 {
	if familyCap == 0 || commandBytes == 0 {
		return 0
	}
	budgetCap := allocationBudget / commandBytes
	if budgetCap == 0 {
		return 0
	}
	if uint64(familyCap) > budgetCap {
		return uint32(budgetCap)
	}
	return familyCap
}

func preparedIndexedMaxCommandsForFamily(family MTLGPUFamily) uint32 {
	if family != MTLGPUFamilyApple7 {
		return 0
	}
	return normalizePreparedIndexedMaxCommands(
		preparedIndexedProvenFamilyCap,
		preparedIndexedSafeAllocationBudget,
		preparedIndexedCommandBytes,
	)
}

func preparedIndexedDispatchGroups(count uint32) uint32 {
	if count == 0 {
		return 0
	}
	return (count + 63) / 64
}
