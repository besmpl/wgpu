//go:build !android && !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import "github.com/gogpu/wgpu/hal"

type platformInstanceState struct{}
type platformSurfaceState struct{}

func newPlatformInstanceState(_ *hal.InstanceDescriptor) (platformInstanceState, error) {
	return platformInstanceState{}, nil
}

func (s *Surface) validatePlatform() error { return nil }
