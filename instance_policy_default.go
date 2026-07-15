//go:build !rust && !android && !(js && wasm)

package wgpu

func validateNativeInstanceDescriptor(_ *InstanceDescriptor) error { return nil }
