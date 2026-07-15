//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"testing"
	"unsafe"

	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func TestAndroidSurfaceCreateInfoPreservesRawNativeWindow(t *testing.T) {
	const (
		generation = uintptr(17)
		window     = uintptr(0x12345678)
	)
	gotGeneration, err := validateAndroidSurfaceRequest(generation, window)
	if err != nil {
		t.Fatalf("validateAndroidSurfaceRequest() error: %v", err)
	}
	info := vk.AndroidSurfaceCreateInfoKHR{SType: vk.StructureTypeAndroidSurfaceCreateInfoKhr}
	setAndroidSurfaceNativeWindow(&info, window)
	if info.SType != vk.StructureTypeAndroidSurfaceCreateInfoKhr {
		t.Fatalf("SType = %v, want VkAndroidSurfaceCreateInfoKHR", info.SType)
	}
	if got := *(*uintptr)(unsafe.Pointer(&info.Window)); got != window {
		t.Fatalf("raw ANativeWindow = %#x, want %#x", got, window)
	}
	if gotGeneration != uint64(generation) {
		t.Fatalf("generation = %d, want %d", gotGeneration, generation)
	}
}

func TestAndroidSurfaceCreateInfoMatchesNDKArm64ABI(t *testing.T) {
	var info vk.AndroidSurfaceCreateInfoKHR
	if got := unsafe.Sizeof(info); got != 32 {
		t.Fatalf("VkAndroidSurfaceCreateInfoKHR size = %d, want 32", got)
	}
	if got := unsafe.Offsetof(info.SType); got != 0 {
		t.Fatalf("sType offset = %d, want 0", got)
	}
	if got := unsafe.Offsetof(info.PNext); got != 8 {
		t.Fatalf("pNext offset = %d, want 8", got)
	}
	if got := unsafe.Offsetof(info.Flags); got != 16 {
		t.Fatalf("flags offset = %d, want 16", got)
	}
	if got := unsafe.Offsetof(info.Window); got != 24 {
		t.Fatalf("window offset = %d, want 24", got)
	}
}

func TestAndroidSurfaceCreateInfoRejectsInvalidHandles(t *testing.T) {
	if _, err := validateAndroidSurfaceRequest(0, 1); err == nil {
		t.Fatal("zero generation was accepted")
	}
	if _, err := validateAndroidSurfaceRequest(1, 0); err == nil {
		t.Fatal("null ANativeWindow was accepted")
	}
}

func TestValidateAndroidSurfaceSupportRequiresEveryCommandClass(t *testing.T) {
	if err := validateAndroidSurfaceSupport(true, true, true); err != nil {
		t.Fatalf("complete Android WSI support rejected: %v", err)
	}
	for _, support := range [][3]bool{{false, true, true}, {true, false, true}, {true, true, false}} {
		if err := validateAndroidSurfaceSupport(support[0], support[1], support[2]); err == nil {
			t.Fatalf("incomplete Android WSI support accepted: %v", support)
		}
	}
}
