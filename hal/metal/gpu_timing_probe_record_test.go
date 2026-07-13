//go:build hearth_gpu_timing_probe

package metal

import "testing"

func TestTimingProbeRecordAggregatesWholeSubmission(t *testing.T) {
	record := timingProbeRecord{expected: 3, valid: true}
	record.addSample(10.0, 11.0, true)
	record.addSample(8.0, 12.0, true)
	record.addSample(9.0, 10.5, true)
	if !record.ready() {
		t.Fatalf("record = %+v, want ready aggregate", record)
	}
	if record.start != 8.0 || record.end != 12.0 {
		t.Fatalf("aggregate = (%v,%v), want (8,12)", record.start, record.end)
	}
}

func TestTimingProbeRecordRejectsIncompleteOrFailedCommandBuffer(t *testing.T) {
	record := timingProbeRecord{expected: 2, valid: true}
	record.addSample(1.0, 2.0, true)
	if record.ready() {
		t.Fatal("incomplete batch reported ready")
	}
	record.addSample(2.0, 1.0, true)
	if record.ready() {
		t.Fatal("inverted timestamps reported ready")
	}

	failed := timingProbeRecord{expected: 1, valid: true}
	failed.addSample(1.0, 2.0, false)
	if failed.ready() {
		t.Fatal("failed command buffer reported GPU timing")
	}
}
