//go:build darwin && hearth_gpu_timing_probe && !rust && !(js && wasm)

package wgpu

// GPUTimingProbeResult is available only in the explicit native timing proof.
// The values come from completed Metal command buffers, not Device.Poll or
// CPU-side command preparation.
type GPUTimingProbeResult struct {
	SubmissionIndex  uint64
	CommandBuffers   uint32
	GPUStartTimeSecs float64
	GPUEndTimeSecs   float64
	DurationNanosecs float64
}

type gpuTimingProbeController interface {
	EnableGPUTimingProbe(capacity int) bool
	DisableGPUTimingProbe()
	GPUTimingProbeResult(submission uint64) (start, end float64, commandBuffers uint32, ready bool)
}

// EnableGPUTimingProbe arms bounded post-completion timing capture on the
// native backend. It is intentionally absent from normal builds.
func (q *Queue) EnableGPUTimingProbe(capacity int) bool {
	if q == nil || q.hal == nil {
		return false
	}
	probe, ok := q.hal.(gpuTimingProbeController)
	return ok && probe.EnableGPUTimingProbe(capacity)
}

// DisableGPUTimingProbe releases the tagged timing state, if any.
func (q *Queue) DisableGPUTimingProbe() {
	if q == nil || q.hal == nil {
		return
	}
	if probe, ok := q.hal.(gpuTimingProbeController); ok {
		probe.DisableGPUTimingProbe()
	}
}

// GPUTimingProbeResult returns one complete submitted batch only after every
// command buffer's completion callback has observed Completed and read its
// GPUStartTime/GPUEndTime values.
func (q *Queue) GPUTimingProbeResult(submission uint64) (GPUTimingProbeResult, bool) {
	if q == nil || q.hal == nil {
		return GPUTimingProbeResult{}, false
	}
	probe, ok := q.hal.(gpuTimingProbeController)
	if !ok {
		return GPUTimingProbeResult{}, false
	}
	start, end, commandBuffers, ready := probe.GPUTimingProbeResult(submission)
	if !ready || end <= start {
		return GPUTimingProbeResult{}, false
	}
	return GPUTimingProbeResult{
		SubmissionIndex:  submission,
		CommandBuffers:   commandBuffers,
		GPUStartTimeSecs: start,
		GPUEndTimeSecs:   end,
		DurationNanosecs: (end - start) * 1e9,
	}, true
}
