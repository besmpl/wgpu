//go:build darwin && !(js && wasm)

package metal

import (
	"testing"

	"github.com/gogpu/gputypes"
)

func TestNormalizePreparedIndexedMaxCommands(t *testing.T) {
	tests := []struct {
		name       string
		familyCap  uint32
		budget     uint64
		commandLen uint64
		want       uint32
	}{
		{name: "family cap", familyCap: 65535, budget: 2 << 20, commandLen: 20, want: 65535},
		{name: "allocation budget", familyCap: 65535, budget: 1 << 20, commandLen: 20, want: 52428},
		{name: "zero budget", familyCap: 10, budget: 19, commandLen: 20, want: 0},
		{name: "zero command size", familyCap: 10, budget: 100, commandLen: 0, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePreparedIndexedMaxCommands(tt.familyCap, tt.budget, tt.commandLen); got != tt.want {
				t.Fatalf("normalized max = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPreparedIndexedMaxCommandsForFamily(t *testing.T) {
	if got := preparedIndexedMaxCommandsForFamily(MTLGPUFamilyApple7); got != 52428 {
		t.Fatalf("Apple7 max = %d, want 52428", got)
	}
	if got := preparedIndexedMaxCommandsForFamily(MTLGPUFamilyApple8); got != 0 {
		t.Fatalf("Apple8 max = %d, want unsupported", got)
	}
}

func TestPreparedIndexedDispatchGroupsRoundUp(t *testing.T) {
	for count, want := range map[uint32]uint32{0: 0, 1: 1, 64: 1, 65: 2, 52428: 820} {
		if got := preparedIndexedDispatchGroups(count); got != want {
			t.Fatalf("count %d groups = %d, want %d", count, got, want)
		}
	}
}

func TestPreparedIndexedEvidenceRequiresOneResetAndDispatch(t *testing.T) {
	evidence := preparedIndexedEvidence{resetCalls: 1, dispatchCalls: 1, dispatchGroups: 2}
	if !evidence.validFor(65) {
		t.Fatal("valid one-reset/one-dispatch evidence rejected")
	}
	evidence.dispatchCalls++
	if evidence.validFor(65) {
		t.Fatal("duplicate dispatch evidence accepted")
	}
}

func TestPreparedIndexedEvidenceRequiresOneExecuteRange(t *testing.T) {
	evidence := preparedIndexedEvidence{executeCalls: 1, executeRange: NSRange{Length: 65}}
	if !evidence.validExecution(65) {
		t.Fatal("valid one-execute range evidence rejected")
	}
	evidence.executeCalls++
	if evidence.validExecution(65) {
		t.Fatal("duplicate execute evidence accepted")
	}
}

func TestPreparedIndexedTranslatorCacheIsBounded(t *testing.T) {
	if preparedIndexedTranslatorShouldCache(preparedIndexedTranslatorCacheLimit-1) != true {
		t.Fatal("cache rejected entry below limit")
	}
	if preparedIndexedTranslatorShouldCache(preparedIndexedTranslatorCacheLimit) != false {
		t.Fatal("cache accepted entry at limit")
	}
}

func TestPreparedIndexedTranslatorReleaseOwnedClearsTokenOwnership(t *testing.T) {
	translator := preparedIndexedTranslator{owned: true}
	translator.releaseOwned()
	if translator != (preparedIndexedTranslator{}) {
		t.Fatalf("owned translator not cleared: %#v", translator)
	}

	cached := preparedIndexedTranslator{library: 1, pipeline: 2}
	cached.releaseOwned()
	if cached.library != 1 || cached.pipeline != 2 || cached.owned {
		t.Fatalf("cached translator was unexpectedly released: %#v", cached)
	}
}

func TestReleasePreparedIndexedTranslatorsClearsCache(t *testing.T) {
	d := &Device{preparedIndexedTranslators: map[preparedIndexedTranslatorKey]preparedIndexedTranslator{
		{format: gputypes.IndexFormatUint16}: {},
	}}
	d.releasePreparedIndexedTranslators()
	if len(d.preparedIndexedTranslators) != 0 {
		t.Fatalf("translator cache retained %d entries", len(d.preparedIndexedTranslators))
	}
}

func TestDeviceDestroyReleasesPreparedIndexedTranslators(t *testing.T) {
	d := &Device{preparedIndexedTranslators: map[preparedIndexedTranslatorKey]preparedIndexedTranslator{
		{format: gputypes.IndexFormatUint32}: {},
	}}
	d.Destroy()
	if len(d.preparedIndexedTranslators) != 0 {
		t.Fatalf("device destroy retained %d translators", len(d.preparedIndexedTranslators))
	}
}

func TestPreparedIndexedArenaCapacityBoundsAndAmortizes(t *testing.T) {
	const max = uint32(52428)
	for _, tc := range []struct {
		count, want uint32
	}{
		{1, 256},
		{20, 256},
		{257, 512},
		{10000, 16384},
		{max, max},
	} {
		if got := preparedIndexedArenaCapacity(tc.count, max); got != tc.want {
			t.Fatalf("arena capacity(%d) = %d, want %d", tc.count, got, tc.want)
		}
	}
	if got := preparedIndexedArenaCapacity(max+1, max); got != 0 {
		t.Fatalf("arena capacity over max = %d, want rejection", got)
	}
}

func TestPreparedIndexedExecutionEvidenceAllowsArenaOffset(t *testing.T) {
	evidence := preparedIndexedEvidence{executeCalls: 1, executeRange: NSRange{Location: 256, Length: 20}}
	if !evidence.validExecutionAt(256, 20) {
		t.Fatal("valid non-zero arena range rejected")
	}
	if evidence.validExecution(20) {
		t.Fatal("non-zero arena range accepted as zero-based")
	}
}

func TestPreparedIndexedArenaReservesDisjointRanges(t *testing.T) {
	arena := &preparedIndexedArena{capacity: 64}
	base0, ok := arena.reserve(20)
	if !ok || base0 != 0 {
		t.Fatalf("first reserve = (%d, %v), want (0, true)", base0, ok)
	}
	base1, ok := arena.reserve(20)
	if !ok || base1 != 20 {
		t.Fatalf("second reserve = (%d, %v), want (20, true)", base1, ok)
	}
	if _, ok := arena.reserve(25); ok {
		t.Fatal("reserve crossed arena capacity")
	}
	if arena.next != 40 {
		t.Fatalf("failed reserve advanced arena to %d, want 40", arena.next)
	}
}
