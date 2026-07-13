//go:build darwin && hearth_prepared_indexed_probe && !(js && wasm)

package metal

import "testing"

func TestPreparedIndexedProbeEvidenceReportsExactRange(t *testing.T) {
	token := &preparedIndexedCommands{evidence: preparedIndexedEvidence{
		resetCalls: 1, dispatchCalls: 1, dispatchGroups: preparedIndexedDispatchGroups(2),
		executeCalls: 1, executeRange: NSRange{Location: 0, Length: 2},
	}}
	reset, dispatch, groups, execute, location, length, ok := token.PreparedIndexedProbeEvidence()
	if !ok || reset != 1 || dispatch != 1 || groups != preparedIndexedDispatchGroups(2) || execute != 1 || location != 0 || length != 2 {
		t.Fatalf("probe evidence = reset=%d dispatch=%d groups=%d execute=%d range=(%d,%d) ok=%t", reset, dispatch, groups, execute, location, length, ok)
	}
}
