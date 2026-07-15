//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func TestValidateVulkanVersion(t *testing.T) {
	tests := []struct {
		name    string
		version uint32
		wantErr bool
	}{
		{name: "below floor", version: vkMakeVersion(1, 1, 99), wantErr: true},
		{name: "at floor", version: vkMakeVersion(1, 2, 0)},
		{name: "above floor", version: vkMakeVersion(1, 3, 7)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateVulkanVersion(test.version)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateVulkanVersion(%#x) error = %v, wantErr %v", test.version, err, test.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "1.1.99") {
				t.Fatalf("error = %q, want reported loader version", err)
			}
		})
	}
}

func TestEnumeratePhysicalDevicesChecksResultsAndReturnedCount(t *testing.T) {
	countCalls, fillCalls := 0, 0
	devices, err := enumeratePhysicalDevicesWith(func(count *uint32, devices *vk.PhysicalDevice) vk.Result {
		if devices == nil {
			countCalls++
			*count = 2
			return vk.Success
		}
		fillCalls++
		if fillCalls == 1 {
			return vk.Incomplete
		}
		*devices = vk.PhysicalDevice(7)
		*count = 1
		return vk.Success
	})
	if err != nil {
		t.Fatalf("enumeratePhysicalDevicesWith() error: %v", err)
	}
	if len(devices) != 1 || devices[0] != 7 || countCalls != 2 || fillCalls != 2 {
		t.Fatalf("devices/calls = (%v, %d, %d), want ([7], 2, 2)", devices, countCalls, fillCalls)
	}

	_, err = enumeratePhysicalDevicesWith(func(_ *uint32, _ *vk.PhysicalDevice) vk.Result {
		return vk.ErrorOutOfHostMemory
	})
	if !errors.Is(err, hal.ErrDeviceOutOfMemory) {
		t.Fatalf("enumeration error = %v, want device out of memory", err)
	}
}

func TestEnumeratePhysicalDevicesRejectsNullHandle(t *testing.T) {
	_, err := enumeratePhysicalDevicesWith(func(count *uint32, devices *vk.PhysicalDevice) vk.Result {
		*count = 1
		return vk.Success
	})
	if err == nil || !strings.Contains(err.Error(), "null device") {
		t.Fatalf("null device error = %v, want fail-closed rejection", err)
	}
}

func TestEnumerateInstanceLayersChecksAndRetries(t *testing.T) {
	fillCalls := 0
	layers, err := enumerateInstanceLayersWith(func(count *uint32, layers *vk.LayerProperties) vk.Result {
		if layers == nil {
			*count = 1
			return vk.Success
		}
		fillCalls++
		if fillCalls == 1 {
			return vk.Incomplete
		}
		copy(layers.LayerName[:], "VK_LAYER_KHRONOS_validation")
		*count = 1
		return vk.Success
	})
	if err != nil {
		t.Fatalf("enumerateInstanceLayersWith() error: %v", err)
	}
	if len(layers) != 1 || fillCalls != 2 {
		t.Fatalf("layers/fill calls = (%d, %d), want (1, 2)", len(layers), fillCalls)
	}
}

func TestSelectInstanceExtensions(t *testing.T) {
	tests := []struct {
		name           string
		available      map[string]struct{}
		debug          bool
		wantExtensions []string
		wantSurface    bool
	}{
		{
			name: "surface pair and debug",
			available: extensionSet(
				"VK_KHR_surface",
				"VK_KHR_android_surface",
				"VK_EXT_debug_utils",
			),
			debug:          true,
			wantExtensions: []string{"VK_KHR_surface\x00", "VK_KHR_android_surface\x00", "VK_EXT_debug_utils\x00"},
			wantSurface:    true,
		},
		{
			name:           "platform dependency missing",
			available:      extensionSet("VK_KHR_android_surface"),
			wantExtensions: nil,
		},
		{
			name:           "headless surface base only",
			available:      extensionSet("VK_KHR_surface"),
			wantExtensions: []string{"VK_KHR_surface\x00"},
		},
		{
			name:           "debug requested but unavailable",
			available:      extensionSet("VK_KHR_surface", "VK_KHR_android_surface"),
			debug:          true,
			wantExtensions: []string{"VK_KHR_surface\x00", "VK_KHR_android_surface\x00"},
			wantSurface:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotExtensions, gotSurface := selectInstanceExtensions(
				test.available,
				"VK_KHR_android_surface\x00",
				test.debug,
			)
			if !reflect.DeepEqual(gotExtensions, test.wantExtensions) {
				t.Fatalf("extensions = %q, want %q", gotExtensions, test.wantExtensions)
			}
			if gotSurface != test.wantSurface {
				t.Fatalf("surfaceEnabled = %v, want %v", gotSurface, test.wantSurface)
			}
		})
	}
}

func TestEnumerateInstanceExtensionsRetriesIncompleteZeroCount(t *testing.T) {
	countCalls := 0
	available, err := enumerateInstanceExtensionsWith(func(count *uint32, properties *vk.ExtensionProperties) vk.Result {
		if properties == nil {
			countCalls++
			if countCalls == 1 {
				return vk.Incomplete
			}
			*count = 1
			return vk.Success
		}
		copy(properties.ExtensionName[:], "VK_KHR_surface")
		*count = 1
		return vk.Success
	})
	if err != nil {
		t.Fatalf("enumerateInstanceExtensionsWith() error: %v", err)
	}
	if _, ok := available["VK_KHR_surface"]; !ok || countCalls != 2 {
		t.Fatalf("available/count calls = (%v, %d), want VK_KHR_surface after retry", available, countCalls)
	}
}

func TestEnumerateInstanceExtensionsPreservesTypedFailure(t *testing.T) {
	_, err := enumerateInstanceExtensionsWith(func(_ *uint32, _ *vk.ExtensionProperties) vk.Result {
		return vk.ErrorOutOfDeviceMemory
	})
	if !errors.Is(err, hal.ErrDeviceOutOfMemory) {
		t.Fatalf("extension enumeration error = %v, want device out of memory", err)
	}
}

func extensionSet(names ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set
}
