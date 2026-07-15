//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"errors"
	"testing"
	"time"

	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func TestDeviceConfiguredSurfaceRegistryDrainsOnce(t *testing.T) {
	device := &Device{handle: 1}
	first := &Surface{}
	second := &Surface{}
	if err := device.registerConfiguredSurface(first); err != nil {
		t.Fatalf("register first surface: %v", err)
	}
	if err := device.registerConfiguredSurface(first); err != nil {
		t.Fatalf("register first surface twice: %v", err)
	}
	if err := device.registerConfiguredSurface(second); err != nil {
		t.Fatalf("register second surface: %v", err)
	}

	surfaces, ok := device.beginDestroy()
	if !ok {
		t.Fatal("first beginDestroy() was rejected")
	}
	if len(surfaces) != 2 {
		t.Fatalf("beginDestroy() returned %d surfaces, want 2 unique surfaces", len(surfaces))
	}
	if _, ok := device.beginDestroy(); ok {
		t.Fatal("second beginDestroy() was accepted")
	}
	if err := device.registerConfiguredSurface(&Surface{}); !errors.Is(err, hal.ErrDeviceLost) {
		t.Fatalf("register during destroy = %v, want device-lost error", err)
	}
}

func TestConfiguredSurfacePublicationIsSerializedWithDeviceDestroy(t *testing.T) {
	device := &Device{handle: 1}
	surface := &Surface{}
	swapchain := &Swapchain{handle: 2, device: device, surface: surface}
	if err := device.registerConfiguredSurface(surface); err != nil {
		t.Fatalf("register surface: %v", err)
	}

	// createSwapchain publishes s.device/s.swapchain while holding this lock.
	// A concurrent Device.Destroy snapshot may already contain the registered
	// surface, but releaseConfiguredDevice must wait for publication to finish.
	surface.mu.Lock()
	done := make(chan struct{})
	go func() {
		surface.releaseConfiguredDevice(device, false)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("device teardown crossed an in-progress surface publication")
	case <-time.After(10 * time.Millisecond):
	}
	surface.device = device
	surface.swapchain = swapchain
	surface.mu.Unlock()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("device teardown did not resume after surface publication")
	}
	if surface.device != nil || surface.swapchain != nil || !swapchain.destroyed {
		t.Fatalf("published swapchain was not abandoned during teardown: surface=%+v swapchain=%+v", surface, swapchain)
	}
}

func TestInstanceLifecycleRegistryOrdersOwnedResources(t *testing.T) {
	instance := &Instance{handle: 1}
	device := &Device{}
	surface := &Surface{}
	if err := instance.registerDevice(device); err != nil {
		t.Fatalf("register device: %v", err)
	}
	if err := instance.registerSurface(surface); err != nil {
		t.Fatalf("register surface: %v", err)
	}
	if err := instance.registerSurface(surface); err != nil {
		t.Fatalf("register surface twice: %v", err)
	}

	surfaces, devices, ok := instance.beginDestroyResources()
	if !ok {
		t.Fatal("first beginDestroyResources() was rejected")
	}
	if len(surfaces) != 1 || surfaces[0] != surface {
		t.Fatalf("owned surfaces = %v, want one registered surface", surfaces)
	}
	if len(devices) != 1 || devices[0] != device {
		t.Fatalf("owned devices = %v, want one registered device", devices)
	}
	if _, _, ok := instance.beginDestroyResources(); ok {
		t.Fatal("second beginDestroyResources() was accepted")
	}
	if err := instance.registerSurface(&Surface{}); err == nil {
		t.Fatal("surface registration during instance destruction was accepted")
	}
	if err := instance.registerDevice(&Device{}); err == nil {
		t.Fatal("device registration during instance destruction was accepted")
	}
}

func TestInstanceDestructionWaitsForResourceAdmission(t *testing.T) {
	instance := &Instance{handle: 1}
	if err := instance.beginResourceCreation(); err != nil {
		t.Fatalf("beginResourceCreation() error: %v", err)
	}

	destroyAdmitted := make(chan struct{})
	go func() {
		instance.creationMu.Lock()
		close(destroyAdmitted)
		instance.creationMu.Unlock()
	}()
	select {
	case <-destroyAdmitted:
		t.Fatal("instance destruction crossed an in-progress native creation")
	case <-time.After(10 * time.Millisecond):
	}
	instance.endResourceCreation()
	select {
	case <-destroyAdmitted:
	case <-time.After(time.Second):
		t.Fatal("instance destruction did not resume after resource registration window")
	}

	if _, _, ok := instance.beginDestroyResources(); !ok {
		t.Fatal("beginDestroyResources() was rejected")
	}
	if err := instance.beginResourceCreation(); err == nil {
		instance.endResourceCreation()
		t.Fatal("resource creation was admitted after instance destruction began")
	}
}

func TestDeviceConfiguredSurfaceRegistryHonorsUnconfigure(t *testing.T) {
	device := &Device{handle: 1}
	surface := &Surface{}
	if err := device.registerConfiguredSurface(surface); err != nil {
		t.Fatalf("register surface: %v", err)
	}
	device.unregisterConfiguredSurface(surface)
	surfaces, ok := device.beginDestroy()
	if !ok {
		t.Fatal("beginDestroy() was rejected")
	}
	if len(surfaces) != 0 {
		t.Fatalf("beginDestroy() returned %d surfaces after unregister, want 0", len(surfaces))
	}
}

func TestReleaseConfiguredDeviceAbandonsHandlesAfterFailedDrain(t *testing.T) {
	device := &Device{handle: 1}
	texture := &SwapchainTexture{handle: 31, view: 32}
	swapchain := &Swapchain{
		handle:             21,
		device:             device,
		images:             []vk.Image{22},
		imageViews:         []vk.ImageView{23},
		acquireSemaphores:  []vk.Semaphore{24},
		presentSemaphores:  []vk.Semaphore{25},
		acquireFenceValues: []uint64{1},
		acquireFence:       26,
		barrierFence:       27,
		barrierPool:        28,
		surfaceTextures:    []*SwapchainTexture{texture},
	}
	texture.swapchain = swapchain
	surface := &Surface{device: device, swapchain: swapchain}
	swapchain.surface = surface

	surface.releaseConfiguredDevice(device, false)

	if surface.device != nil || surface.swapchain != nil {
		t.Fatalf("surface retained destroyed device state: %+v", surface)
	}
	if !swapchain.destroyed || !swapchain.broken || !errors.Is(swapchain.failureErr, hal.ErrDeviceLost) {
		t.Fatalf("abandoned swapchain state = destroyed %v broken %v error %v", swapchain.destroyed, swapchain.broken, swapchain.failureErr)
	}
	if swapchain.handle != 0 || swapchain.device != nil || len(swapchain.images) != 0 {
		t.Fatalf("abandoned swapchain retained native handles: %+v", swapchain)
	}
	if texture.handle != 0 || texture.view != 0 || texture.swapchain != nil {
		t.Fatalf("external swapchain texture retained native handles: %+v", texture)
	}
}
