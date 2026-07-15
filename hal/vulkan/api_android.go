//go:build android && arm64

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"

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
	generation, err := validateAndroidSurfaceRequest(displayHandle, windowHandle)
	if err != nil {
		return nil, err
	}
	if err := i.beginResourceCreation(); err != nil {
		return nil, err
	}
	defer i.endResourceCreation()
	if err := validateAndroidSurfaceSupport(i.surfaceEnabled, i.cmds.HasWSIQueries(), i.cmds.HasCreateAndroidSurfaceKHR()); err != nil {
		return nil, err
	}
	if !i.platform.canCreateSurface(generation) {
		return nil, hal.ErrSurfaceLost
	}
	createInfo := vk.AndroidSurfaceCreateInfoKHR{
		SType: vk.StructureTypeAndroidSurfaceCreateInfoKhr,
	}
	setAndroidSurfaceNativeWindow(&createInfo, windowHandle)

	var handle vk.SurfaceKHR
	result := i.cmds.CreateAndroidSurfaceKHR(i.handle, &createInfo, nil, &handle)
	if result != vk.Success {
		return nil, mapAndroidSurfaceCreateError(result)
	}
	if handle == 0 {
		return nil, fmt.Errorf("vulkan: vkCreateAndroidSurfaceKHR returned success with a null surface")
	}

	if !i.platform.commitSurface(generation) {
		i.cmds.DestroySurfaceKHR(i.handle, handle, nil)
		return nil, hal.ErrSurfaceLost
	}
	return i.adoptSurface(&Surface{
		handle:   handle,
		instance: i,
		platform: platformSurfaceState{generation: generation},
	})
}
