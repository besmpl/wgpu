//go:build android && arm64

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// platformSurfaceExtension returns the Android Vulkan WSI extension.
func platformSurfaceExtension() string {
	return "VK_KHR_android_surface\x00"
}

// CreateSurface creates a Vulkan surface from an ANativeWindow.
//
// displayHandle is a non-zero, monotonically increasing host window
// generation. windowHandle is the raw ANativeWindow pointer. The host retains
// its application reference. Vulkan acquires a surface reference on successful
// creation and releases it from vkDestroySurfaceKHR; WGPU must not call
// ANativeWindow_acquire or ANativeWindow_release itself.
func (i *Instance) CreateSurface(displayHandle, windowHandle uintptr) (hal.Surface, error) {
	if displayHandle == 0 {
		return nil, fmt.Errorf("vulkan: Android window generation must be non-zero")
	}
	if windowHandle == 0 {
		return nil, fmt.Errorf("vulkan: Android ANativeWindow must be non-null")
	}
	if !i.surfaceEnabled || !i.cmds.HasWSIQueries() || !i.cmds.HasCreateAndroidSurfaceKHR() {
		return nil, fmt.Errorf("vulkan: Android surface WSI is unavailable")
	}
	if !i.platform.canCreateSurface(uint64(displayHandle)) {
		return nil, hal.ErrSurfaceLost
	}

	createInfo := vk.AndroidSurfaceCreateInfoKHR{
		SType: vk.StructureTypeAndroidSurfaceCreateInfoKhr,
	}
	// Window is generated as *vk.ANativeWindow, but contains a raw C pointer.
	// Store the uintptr value in-place without converting it into a Go pointer.
	*(*uintptr)(unsafe.Pointer(&createInfo.Window)) = windowHandle

	var handle vk.SurfaceKHR
	result := i.cmds.CreateAndroidSurfaceKHR(i.handle, &createInfo, nil, &handle)
	if result != vk.Success {
		return nil, mapAndroidSurfaceCreateError(result)
	}
	if handle == 0 {
		return nil, fmt.Errorf("vulkan: vkCreateAndroidSurfaceKHR returned success with a null surface")
	}

	generation := uint64(displayHandle)
	if !i.platform.commitSurface(generation) {
		i.cmds.DestroySurfaceKHR(i.handle, handle, nil)
		return nil, hal.ErrSurfaceLost
	}
	return &Surface{
		handle:   handle,
		instance: i,
		platform: platformSurfaceState{generation: generation},
	}, nil
}

func mapAndroidSurfaceCreateError(result vk.Result) error {
	switch result {
	case vk.ErrorSurfaceLostKhr, vk.ErrorNativeWindowInUseKhr:
		return hal.ErrSurfaceLost
	default:
		return fmt.Errorf("vulkan: vkCreateAndroidSurfaceKHR failed: %d", result)
	}
}
