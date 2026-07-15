//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// swapchainPlatformPolicy contains the platform-specific choices required by
// the otherwise shared Vulkan swapchain mechanism. Keeping these decisions in
// a value makes the Android API-level behavior testable on every host.
type swapchainPlatformPolicy struct {
	android           bool
	androidSDKVersion uint32
}

func defaultSwapchainPlatformPolicy() swapchainPlatformPolicy {
	return swapchainPlatformPolicy{}
}

func androidSwapchainPlatformPolicy(sdk uint32) swapchainPlatformPolicy {
	return swapchainPlatformPolicy{android: true, androidSDKVersion: sdk}
}

func (p swapchainPlatformPolicy) acquireTimeout(requested uint64) uint64 {
	// Android 10's presentation implementation does not support finite acquire
	// timeouts. Android 11 (API 30) added support for them.
	if p.android && p.androidSDKVersion < 30 {
		return ^uint64(0)
	}
	return requested
}

func (p swapchainPlatformPolicy) preTransform(capabilities vk.SurfaceCapabilitiesKHR) (vk.SurfaceTransformFlagBitsKHR, error) {
	transform := capabilities.CurrentTransform
	if p.android {
		// Match Rust wgpu's Android policy: let the compositor pre-rotate by
		// creating the swapchain with the identity transform.
		transform = vk.SurfaceTransformIdentityBitKhr
	}
	if transform == 0 {
		return 0, fmt.Errorf("vulkan: surface returned no usable transform")
	}
	if vk.Flags(capabilities.SupportedTransforms)&vk.Flags(transform) == 0 {
		if p.android {
			return 0, fmt.Errorf("vulkan: Android surface does not support the required identity transform")
		}
		return 0, fmt.Errorf("vulkan: surface current transform is not supported")
	}
	return transform, nil
}

func (p swapchainPlatformPolicy) reportSuboptimal(suboptimal bool) bool {
	// Android reports SUBOPTIMAL when identity pre-transform differs from the
	// current device orientation. Rust wgpu intentionally treats that as
	// success for both acquire and present.
	return suboptimal && !p.android
}

func swapchainPolicyForSurface(surface *Surface) swapchainPlatformPolicy {
	if surface == nil || surface.instance == nil {
		return defaultSwapchainPlatformPolicy()
	}
	return surface.instance.platform.swapchain
}

func mapSwapchainCreateResult(result vk.Result) error {
	switch result {
	case vk.ErrorSurfaceLostKhr, vk.ErrorInitializationFailed:
		return fmt.Errorf("vulkan: vkCreateSwapchainKHR failed: %w", hal.ErrSurfaceLost)
	case vk.ErrorNativeWindowInUseKhr:
		return fmt.Errorf("vulkan: native window is already in use by another swapchain")
	default:
		return mapVulkanResult("vkCreateSwapchainKHR", result)
	}
}

func mapAndroidSurfaceCreateError(result vk.Result) error {
	switch result {
	case vk.ErrorSurfaceLostKhr, vk.ErrorNativeWindowInUseKhr, vk.ErrorInitializationFailed:
		return fmt.Errorf("vulkan: vkCreateAndroidSurfaceKHR failed: %w", hal.ErrSurfaceLost)
	default:
		return mapVulkanResult("vkCreateAndroidSurfaceKHR", result)
	}
}

// mapVulkanResult preserves the typed HAL errors that callers use to recover
// from Vulkan failures. WSI operations share this table so an out-of-memory or
// device-lost result cannot accidentally degrade into an opaque numeric error.
func mapVulkanResult(operation string, result vk.Result) error {
	switch result {
	case vk.Success:
		return nil
	case vk.Timeout:
		return fmt.Errorf("vulkan: %s failed: %w", operation, hal.ErrTimeout)
	case vk.NotReady:
		return fmt.Errorf("vulkan: %s failed: %w", operation, hal.ErrNotReady)
	case vk.ErrorOutOfHostMemory, vk.ErrorOutOfDeviceMemory:
		return fmt.Errorf("vulkan: %s failed: %w", operation, hal.ErrDeviceOutOfMemory)
	case vk.ErrorDeviceLost:
		return fmt.Errorf("vulkan: %s failed: %w", operation, hal.ErrDeviceLost)
	case vk.ErrorSurfaceLostKhr:
		return fmt.Errorf("vulkan: %s failed: %w", operation, hal.ErrSurfaceLost)
	case vk.ErrorOutOfDateKhr:
		return fmt.Errorf("vulkan: %s failed: %w", operation, hal.ErrSurfaceOutdated)
	default:
		return fmt.Errorf("vulkan: %s failed: %d", operation, result)
	}
}
