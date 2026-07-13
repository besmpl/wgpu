//go:build darwin && hearth_prepared_indexed_probe && !(js && wasm)

package wgpu

import "github.com/besmpl/wgpu/core"

// PreparedIndexedProbeEvidence forwards tagged native evidence without
// widening the ordinary wgpu prepared-indexed surface.
func PreparedIndexedProbeEvidence(token *PreparedIndexedCommands) (reset, dispatch, groups, execute uint32, location, length uint64, ok bool) {
	if token == nil || token.state == nil {
		return 0, 0, 0, 0, 0, 0, false
	}
	state, ok := token.state.(*preparedIndexedState)
	if !ok || state == nil {
		return 0, 0, 0, 0, 0, 0, false
	}
	return core.PreparedIndexedProbeEvidence(state.core)
}
