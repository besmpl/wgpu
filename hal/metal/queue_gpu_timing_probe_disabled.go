//go:build darwin && !(js && wasm) && !hearth_gpu_timing_probe

package metal

// These helpers deliberately compile to no-ops outside the opt-in timing
// proof. Keeping the calls in Queue.Submit lets the tagged build observe the
// exact submitted batch while the normal queue has no timing state or work.
func timingProbeBeginSubmission(*Queue, uint64, uint32) {}

func timingProbeAttachCommandBuffer(*Queue, ID, uint64) {}
