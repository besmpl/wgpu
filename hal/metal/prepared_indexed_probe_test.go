//go:build darwin && hearth_prepared_indexed_probe && !(js && wasm)

package metal

import "testing"

// TestPreparedIndexedProbeLifecycle is deliberately opt-in. It verifies the
// exact family/selector gate and that a real MTLIndirectCommandBuffer plus the
// raw-MSL translator pipeline can be created and torn down on this machine.
// The renderer's tagged proof owns the stronger two-record visible readback.
func TestPreparedIndexedProbeLifecycle(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("metal init: %v", err)
	}
	devices := CopyAllDevices()
	if len(devices) == 0 {
		t.Skip("no Metal devices available")
	}
	defer func() {
		for _, device := range devices {
			Release(device)
		}
	}()
	for _, raw := range devices {
		t.Run(DeviceName(raw), func(t *testing.T) {
			if !preparedIndexedProvenFamily(raw) {
				t.Skip("unproved prepared-indexed Metal GPU family")
			}
			if err := preparedIndexedSelectorCheck(raw); err != nil {
				t.Fatal(err)
			}
			device, err := newDevice(&Adapter{raw: raw})
			if err != nil {
				t.Fatalf("new device: %v", err)
			}
			defer device.Destroy()
			if err := preparedIndexedICBTranslatorProbe(device); err != nil {
				t.Fatal(err)
			}
		})
	}
}
