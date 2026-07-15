//go:build !rust && android

package wgpu

func validateNativeInstanceDescriptor(desc *InstanceDescriptor) error {
	if desc == nil {
		return nil
	}
	return validateAndroidInstanceFlags(desc.Flags)
}
