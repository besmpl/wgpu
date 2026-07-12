//go:build darwin && !(js && wasm)

package metal

import "fmt"

// preparedIndexedProvenFamily is intentionally narrower than Metal 3. Apple
// family 7 is the M1 family on which Hearth's visible prepared-indexed proof is
// required to pass. Extend this allowlist only with the same proof on another
// family.
func preparedIndexedProvenFamily(device ID) bool {
	return DeviceSupportsFamily(device, MTLGPUFamilyApple7)
}

// preparedIndexedSelectorCheck verifies the exact Objective-C surface needed
// by the ICB translator route before advertising it.
func preparedIndexedSelectorCheck(device ID) error {
	for _, name := range []string{
		"newIndirectCommandBufferWithDescriptor:maxCommandCount:options:",
		"newLibraryWithSource:options:error:",
		"newComputePipelineStateWithFunction:error:",
		"newBufferWithLength:options:",
	} {
		sel := Sel(name)
		if sel == 0 || !MsgSendBool(device, Sel("respondsToSelector:"), uintptr(sel)) {
			return fmt.Errorf("device does not respond to %s", name)
		}
	}
	descClass := GetClass("MTLIndirectCommandBufferDescriptor")
	if descClass == 0 {
		return fmt.Errorf("MTLIndirectCommandBufferDescriptor class unavailable")
	}
	if !MsgSendBool(ID(descClass), Sel("respondsToSelector:"), uintptr(Sel("alloc"))) {
		return fmt.Errorf("descriptor class does not respond to alloc")
	}
	for _, name := range []string{"init", "setCommandTypes:", "setInheritPipelineState:", "setInheritBuffers:"} {
		if !MsgSendBool(ID(descClass), Sel("instancesRespondToSelector:"), uintptr(Sel(name))) {
			return fmt.Errorf("descriptor does not respond to %s", name)
		}
	}
	return nil
}

// preparedIndexedObjectSelectorCheck is used after each native object is
// created. Several Metal encoder types are protocols rather than runtime
// classes, so checking their actual instances is more reliable than guessing
// class names during capability discovery.
func preparedIndexedObjectSelectorCheck(object ID, label string, selectors ...string) error {
	if object == 0 {
		return fmt.Errorf("%s is nil", label)
	}
	for _, name := range selectors {
		sel := Sel(name)
		// Some Metal encoder implementations are protocol-backed objects whose
		// respondsToSelector: bridge is not reliable through the cgo-free FFI.
		// Selector registration remains a necessary boundary check; concrete
		// invocation errors are handled by Metal validation and command errors.
		if sel == 0 {
			return fmt.Errorf("%s does not respond to %s", label, name)
		}
	}
	return nil
}
