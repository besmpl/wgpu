//go:build darwin && !hearth_prepared_indexed_probe && !(js && wasm)

package metal

import "github.com/gogpu/wgpu/hal"

// The Apple 7 GPU family/selector route is promoted after the tagged M1
// two-record render/readback proof. Do not infer support from the broader
// Metal 3 family: that would advertise unproved Intel, AMD, and newer Apple
// routes. Keep capability discovery allocation-free; translator and ICB
// creation failures remain ordinary prepare-time errors.
func preparedIndexedCapabilities(d *Device) hal.PreparedIndexedCapabilities {
	if d == nil || d.raw == 0 || !preparedIndexedProvenFamily(d.raw) {
		return hal.PreparedIndexedCapabilities{}
	}
	if err := preparedIndexedSelectorCheck(d.raw); err != nil {
		return hal.PreparedIndexedCapabilities{}
	}
	return hal.PreparedIndexedCapabilities{PreparedIndexed: true, MaxCommands: preparedIndexedMaxCommandsForFamily(MTLGPUFamilyApple7)}
}
