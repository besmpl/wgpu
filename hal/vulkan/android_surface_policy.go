//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func validateAndroidSurfaceSupport(surfaceEnabled, hasWSIQueries, hasCreateCommand bool) error {
	if !surfaceEnabled || !hasWSIQueries || !hasCreateCommand {
		return fmt.Errorf("vulkan: Android surface WSI is unavailable")
	}
	return nil
}

func validateAndroidSurfaceRequest(generation, window uintptr) (uint64, error) {
	if generation == 0 {
		return 0, fmt.Errorf("vulkan: Android window generation must be non-zero")
	}
	if window == 0 {
		return 0, fmt.Errorf("vulkan: Android ANativeWindow must be non-null")
	}
	return uint64(generation), nil
}

func setAndroidSurfaceNativeWindow(createInfo *vk.AndroidSurfaceCreateInfoKHR, window uintptr) {
	// Window is generated as *vk.ANativeWindow but contains a raw C pointer.
	// Store the uintptr value in-place without converting it to a Go pointer.
	*(*uintptr)(unsafe.Pointer(&createInfo.Window)) = window
}
