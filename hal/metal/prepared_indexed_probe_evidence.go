//go:build darwin && hearth_prepared_indexed_probe && !(js && wasm)

package metal

// PreparedIndexedProbeEvidence exposes the already-recorded narrow HAL trace
// only to the tagged proof bridge. Production builds do not compile this
// method or its callers.
func (p *preparedIndexedCommands) PreparedIndexedProbeEvidence() (reset, dispatch, groups, execute uint32, location, length uint64, ok bool) {
	if p == nil {
		return 0, 0, 0, 0, 0, 0, false
	}
	return p.evidence.resetCalls, p.evidence.dispatchCalls, p.evidence.dispatchGroups,
		p.evidence.executeCalls, uint64(p.evidence.executeRange.Location), uint64(p.evidence.executeRange.Length), true
}
