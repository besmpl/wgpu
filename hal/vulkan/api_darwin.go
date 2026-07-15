//go:build darwin && !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"unsafe"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// platformSurfaceExtension returns the macOS surface extension.
func platformSurfaceExtension() string {
	return "VK_EXT_metal_surface\x00"
}

// CreateSurface creates a Metal surface from a CAMetalLayer.
// Parameters:
//   - _: unused first parameter for API consistency with other platforms
//   - metalLayer: Pointer to CAMetalLayer
func (i *Instance) CreateSurface(_, metalLayer uintptr) (hal.Surface, error) {
	if err := i.beginResourceCreation(); err != nil {
		return nil, err
	}
	defer i.endResourceCreation()
	if !i.surfaceEnabled || !i.cmds.HasWSIQueries() || !i.cmds.HasCreateMetalSurfaceEXT() {
		return nil, fmt.Errorf("vulkan: Metal surface WSI is unavailable")
	}
	createInfo := vk.MetalSurfaceCreateInfoEXT{
		SType: vk.StructureTypeMetalSurfaceCreateInfoExt,
	}
	// Write CAMetalLayer* value directly into the PLayer field memory.
	// PLayer is *CAMetalLayer (a Go pointer type) but must hold the raw C pointer
	// address. We cannot use unsafe.Pointer(uintptr) — go vet rejects it.
	// Instead, write the uintptr value into the field's memory location.
	// Previous bug: &metalLayer stored Go stack address instead of CAMetalLayer* value.
	*(*uintptr)(unsafe.Pointer(&createInfo.PLayer)) = metalLayer

	var surface vk.SurfaceKHR
	result := i.cmds.CreateMetalSurfaceEXT(i.handle, &createInfo, nil, &surface)
	if result != vk.Success {
		return nil, fmt.Errorf("vulkan: vkCreateMetalSurfaceEXT failed: %d", result)
	}
	if surface == 0 {
		return nil, fmt.Errorf("vulkan: vkCreateMetalSurfaceEXT returned success but surface is null")
	}

	return i.adoptSurface(&Surface{
		handle:   surface,
		instance: i,
	})
}
