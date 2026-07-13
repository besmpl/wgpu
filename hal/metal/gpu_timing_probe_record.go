//go:build hearth_gpu_timing_probe

package metal

import "math"

// timingProbeRecord is one bounded submission's aggregate. Metal reports
// command-buffer timestamps in seconds. The aggregate uses the earliest start
// and latest end across every command buffer in the submitted batch.
type timingProbeRecord struct {
	expected  uint32
	completed uint32
	valid     bool
	hasSample bool
	start     float64
	end       float64
}

func (record *timingProbeRecord) addSample(start, end float64, completed bool) {
	if record == nil {
		return
	}
	record.completed++
	if !completed || math.IsNaN(start) || math.IsNaN(end) || math.IsInf(start, 0) || math.IsInf(end, 0) || end <= start {
		record.valid = false
		return
	}
	if !record.hasSample || start < record.start {
		record.start = start
	}
	if !record.hasSample || end > record.end {
		record.end = end
	}
	record.hasSample = true
}

func (record *timingProbeRecord) ready() bool {
	return record != nil && record.expected > 0 && record.completed == record.expected && record.valid && record.hasSample && record.end > record.start
}
