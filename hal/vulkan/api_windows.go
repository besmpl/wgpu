//go:build windows && !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"syscall"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	getModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

// platformSurfaceExtension returns the Windows surface extension.
func platformSurfaceExtension() string {
	return "VK_KHR_win32_surface\x00"
}

// CreateSurface creates a Windows surface from HINSTANCE and HWND.
func (i *Instance) CreateSurface(hinstance, hwnd uintptr) (hal.Surface, error) {
	if err := i.beginResourceCreation(); err != nil {
		return nil, err
	}
	defer i.endResourceCreation()
	if !i.surfaceEnabled || !i.cmds.HasWSIQueries() || !i.cmds.HasCreateWin32SurfaceKHR() {
		return nil, fmt.Errorf("vulkan: Windows surface WSI is unavailable")
	}
	// If hinstance is 0, get the current module handle
	if hinstance == 0 {
		hinstance, _, _ = getModuleHandleW.Call(0)
	}

	createInfo := vk.Win32SurfaceCreateInfoKHR{
		SType:     vk.StructureTypeWin32SurfaceCreateInfoKhr,
		Hinstance: hinstance,
		Hwnd:      hwnd,
	}

	var surface vk.SurfaceKHR
	result := i.cmds.CreateWin32SurfaceKHR(i.handle, &createInfo, nil, &surface)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateWin32SurfaceKHR failed: %d", result)
	}
	if surface == 0 {
		return nil, fmt.Errorf("vulkan: vkCreateWin32SurfaceKHR returned success but surface is null")
	}

	return i.adoptSurface(&Surface{
		handle:   surface,
		instance: i,
	})
}
