//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"reflect"
	"strings"
	"testing"
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

func extensionSet(names ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}
	return set
}
