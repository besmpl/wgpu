//go:build darwin && hearth_prepared_indexed_probe && !(js && wasm)

package core

// preparedIndexedProbeEvidenceProvider is implemented only by the tagged
// Metal evidence shim. Keeping the tuple private to this build avoids adding
// a runtime/public evidence surface to the prepared-indexed API.
type preparedIndexedProbeEvidenceProvider interface {
	PreparedIndexedProbeEvidence() (reset, dispatch, groups, execute uint32, location, length uint64, ok bool)
}

// PreparedIndexedProbeEvidence returns backend evidence for one prepared
// token. It is intentionally available only to the tagged native proof.
func PreparedIndexedProbeEvidence(token *PreparedIndexedCommands) (reset, dispatch, groups, execute uint32, location, length uint64, ok bool) {
	if token == nil || token.prepared == nil {
		return 0, 0, 0, 0, 0, 0, false
	}
	provider, ok := token.prepared.(preparedIndexedProbeEvidenceProvider)
	if !ok {
		return 0, 0, 0, 0, 0, 0, false
	}
	return provider.PreparedIndexedProbeEvidence()
}
