//go:build !(js && wasm)

package vulkan

import (
	"strings"
	"testing"

	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

func TestEnumerateDeviceExtensionsChecksCountResult(t *testing.T) {
	_, err := enumerateDeviceExtensionsWith(func(_ *uint32, _ *vk.ExtensionProperties) vk.Result {
		return vk.ErrorInitializationFailed
	})
	if err == nil || !strings.Contains(err.Error(), "count query") {
		t.Fatalf("count query error = %v, want checked count-query failure", err)
	}
}

func TestEnumerateDeviceExtensionsChecksPropertyResult(t *testing.T) {
	_, err := enumerateDeviceExtensionsWith(func(count *uint32, properties *vk.ExtensionProperties) vk.Result {
		if properties == nil {
			*count = 1
			return vk.Success
		}
		return vk.ErrorInitializationFailed
	})
	if err == nil || !strings.Contains(err.Error(), "vkEnumerateDeviceExtensionProperties failed") {
		t.Fatalf("property query error = %v, want checked property-query failure", err)
	}
}

func TestEnumerateDeviceExtensionsRetriesIncomplete(t *testing.T) {
	countCalls := 0
	fillCalls := 0
	available, err := enumerateDeviceExtensionsWith(func(count *uint32, properties *vk.ExtensionProperties) vk.Result {
		if properties == nil {
			countCalls++
			*count = 1
			return vk.Success
		}
		fillCalls++
		if fillCalls == 1 {
			return vk.Incomplete
		}
		copy(properties.ExtensionName[:], swapchainExtension)
		*count = 1
		return vk.Success
	})
	if err != nil {
		t.Fatalf("enumerateDeviceExtensionsWith() error: %v", err)
	}
	if _, ok := available[swapchainExtension]; !ok {
		t.Fatalf("available extensions = %v, want %s", available, swapchainExtension)
	}
	if countCalls != 2 || fillCalls != 2 {
		t.Fatalf("query calls = (count %d, fill %d), want (2, 2)", countCalls, fillCalls)
	}
}

func TestEnumerateDeviceExtensionsRejectsUnstableCount(t *testing.T) {
	countCalls := 0
	fillCalls := 0
	_, err := enumerateDeviceExtensionsWith(func(count *uint32, properties *vk.ExtensionProperties) vk.Result {
		if properties == nil {
			countCalls++
			*count = 1
			return vk.Success
		}
		fillCalls++
		return vk.Incomplete
	})
	if err == nil || !strings.Contains(err.Error(), "unstable count") {
		t.Fatalf("unstable enumeration error = %v, want bounded retry failure", err)
	}
	if countCalls != deviceExtensionQueryRetries || fillCalls != deviceExtensionQueryRetries {
		t.Fatalf("query calls = (count %d, fill %d), want (%d, %d)",
			countCalls, fillCalls, deviceExtensionQueryRetries, deviceExtensionQueryRetries)
	}
}

func TestEnumerateDeviceExtensionsRetriesOversizedReturnedCount(t *testing.T) {
	fillCalls := 0
	available, err := enumerateDeviceExtensionsWith(func(count *uint32, properties *vk.ExtensionProperties) vk.Result {
		if properties == nil {
			*count = 1
			return vk.Success
		}
		fillCalls++
		if fillCalls == 1 {
			*count = 2
			return vk.Success
		}
		copy(properties.ExtensionName[:], swapchainExtension)
		*count = 1
		return vk.Success
	})
	if err != nil {
		t.Fatalf("enumerateDeviceExtensionsWith() error: %v", err)
	}
	if _, ok := available[swapchainExtension]; !ok || fillCalls != 2 {
		t.Fatalf("available/fill calls = (%v, %d), want %s after retry", available, fillCalls, swapchainExtension)
	}
}

func TestSelectDeviceExtensionsRequiresSwapchain(t *testing.T) {
	_, _, err := selectDeviceExtensions(map[string]struct{}{
		incrementalPresentExtension: {},
	})
	if err == nil || !strings.Contains(err.Error(), swapchainExtension) {
		t.Fatalf("missing swapchain error = %v, want required-extension failure", err)
	}
}

func TestSelectDeviceExtensionsEnablesAdvertisedExtensions(t *testing.T) {
	extensions, incremental, err := selectDeviceExtensions(map[string]struct{}{
		swapchainExtension:          {},
		incrementalPresentExtension: {},
	})
	if err != nil {
		t.Fatalf("selectDeviceExtensions() error: %v", err)
	}
	want := []string{swapchainExtension + "\x00", incrementalPresentExtension + "\x00"}
	if len(extensions) != len(want) || extensions[0] != want[0] || extensions[1] != want[1] {
		t.Fatalf("extensions = %q, want %q", extensions, want)
	}
	if !incremental {
		t.Fatal("incremental present support was not reported")
	}
}
