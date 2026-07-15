//go:build android && arm64

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
)

type platformInstanceState struct {
	windows   *windowGenerationState
	swapchain swapchainPlatformPolicy
}

type platformSurfaceState struct {
	generation uint64
}

func newPlatformInstanceState(desc *hal.InstanceDescriptor) (platformInstanceState, error) {
	if desc != nil && desc.Flags&gputypes.InstanceFlagsDebug != 0 {
		return platformInstanceState{}, fmt.Errorf("vulkan: debug callbacks are unsupported on Android")
	}
	sdk, err := queryAndroidSDKVersion()
	if err != nil {
		return platformInstanceState{}, fmt.Errorf("vulkan: query Android SDK version: %w", err)
	}
	if sdk < 29 {
		return platformInstanceState{}, fmt.Errorf("vulkan: Android API 29 or newer is required, device reports %d", sdk)
	}
	return platformInstanceState{
		windows:   &windowGenerationState{},
		swapchain: androidSwapchainPlatformPolicy(sdk),
	}, nil
}

var (
	androidSDKVersionOnce sync.Once
	androidSDKVersion     uint32
	androidSDKVersionErr  error
)

func queryAndroidSDKVersion() (uint32, error) {
	androidSDKVersionOnce.Do(func() {
		androidSDKVersion, androidSDKVersionErr = loadAndroidSDKVersion()
	})
	return androidSDKVersion, androidSDKVersionErr
}

func loadAndroidSDKVersion() (uint32, error) {
	handle, err := ffi.LoadLibrary("libc.so")
	if err != nil {
		return 0, err
	}
	defer func() { _ = ffi.FreeLibrary(handle) }()
	fn, err := ffi.GetSymbol(handle, "android_get_device_api_level")
	if err != nil {
		return 0, err
	}
	var call types.CallInterface
	if err := ffi.PrepareCallInterface(&call, types.DefaultCall, types.SInt32TypeDescriptor, nil); err != nil {
		return 0, err
	}
	var sdk int32
	if _, err := ffi.CallFunction(&call, fn, unsafe.Pointer(&sdk), nil); err != nil {
		return 0, err
	}
	if sdk < 0 {
		return 0, fmt.Errorf("invalid negative SDK version %d", sdk)
	}
	return uint32(sdk), nil
}

func (p *platformInstanceState) canCreateSurface(generation uint64) bool {
	if p == nil || p.windows == nil {
		return false
	}
	return p.windows.canCreate(generation)
}

func (p *platformInstanceState) commitSurface(generation uint64) bool {
	if p == nil || p.windows == nil {
		return false
	}
	return p.windows.commit(generation)
}

func (p *platformInstanceState) validateSurface(generation uint64) error {
	if p == nil || p.windows == nil || !p.windows.isCurrent(generation) {
		return hal.ErrSurfaceLost
	}
	return nil
}

func (s *Surface) validatePlatform() error {
	if s == nil || s.instance == nil {
		return hal.ErrSurfaceLost
	}
	return s.instance.platform.validateSurface(s.platform.generation)
}
