//go:build !(js && wasm)

package vk

import (
	"strings"
	"testing"
	"unsafe"
)

func TestRequireSwapchainCommandsAcceptsCompleteSet(t *testing.T) {
	commands := commandsWithSwapchainEntryPoints()
	if !commands.HasSwapchainCommands() {
		t.Fatal("complete VK_KHR_swapchain command set was rejected")
	}
	if err := commands.requireSwapchainCommands(); err != nil {
		t.Fatalf("requireSwapchainCommands() error: %v", err)
	}
}

func TestRequireSwapchainCommandsNamesEveryMissingEntryPoint(t *testing.T) {
	tests := []struct {
		name  string
		clear func(*Commands)
	}{
		{"vkCreateSwapchainKHR", func(commands *Commands) { commands.createSwapchainKHR = nil }},
		{"vkDestroySwapchainKHR", func(commands *Commands) { commands.destroySwapchainKHR = nil }},
		{"vkGetSwapchainImagesKHR", func(commands *Commands) { commands.getSwapchainImagesKHR = nil }},
		{"vkAcquireNextImageKHR", func(commands *Commands) { commands.acquireNextImageKHR = nil }},
		{"vkQueuePresentKHR", func(commands *Commands) { commands.queuePresentKHR = nil }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			commands := commandsWithSwapchainEntryPoints()
			test.clear(commands)
			if commands.HasSwapchainCommands() {
				t.Fatal("incomplete VK_KHR_swapchain command set was accepted")
			}
			err := commands.requireSwapchainCommands()
			if err == nil || !strings.Contains(err.Error(), test.name) {
				t.Fatalf("requireSwapchainCommands() error = %v, want missing command %s", err, test.name)
			}
			if !strings.Contains(err.Error(), "VK_KHR_swapchain") {
				t.Fatalf("requireSwapchainCommands() error = %v, want extension context", err)
			}
		})
	}
}

func commandsWithSwapchainEntryPoints() *Commands {
	pointer := unsafe.Pointer(new(byte))
	return &Commands{
		createSwapchainKHR:    pointer,
		destroySwapchainKHR:   pointer,
		getSwapchainImagesKHR: pointer,
		acquireNextImageKHR:   pointer,
		queuePresentKHR:       pointer,
	}
}

func TestHasCreateAndroidSurfaceKHRRequiresEntryPoint(t *testing.T) {
	commands := &Commands{}
	if commands.HasCreateAndroidSurfaceKHR() {
		t.Fatal("nil vkCreateAndroidSurfaceKHR entry point was accepted")
	}
	commands.createAndroidSurfaceKHR = unsafe.Pointer(new(byte))
	if !commands.HasCreateAndroidSurfaceKHR() {
		t.Fatal("loaded vkCreateAndroidSurfaceKHR entry point was rejected")
	}
}

func TestRequireCoreInstanceCommandsNamesMissingEntryPoint(t *testing.T) {
	pointer := unsafe.Pointer(new(byte))
	commands := &Commands{
		destroyInstance:                        pointer,
		enumeratePhysicalDevices:               pointer,
		getPhysicalDeviceProperties:            pointer,
		getPhysicalDeviceQueueFamilyProperties: pointer,
		getPhysicalDeviceMemoryProperties:      pointer,
		getPhysicalDeviceFeatures:              pointer,
		getPhysicalDeviceFormatProperties:      pointer,
		enumerateDeviceExtensionProperties:     pointer,
		createDevice:                           pointer,
	}
	if err := commands.requireCoreInstanceCommands(); err != nil {
		t.Fatalf("complete core instance command set rejected: %v", err)
	}
	commands.getPhysicalDeviceMemoryProperties = nil
	if err := commands.requireCoreInstanceCommands(); err == nil || !strings.Contains(err.Error(), "vkGetPhysicalDeviceMemoryProperties") {
		t.Fatalf("missing core command error = %v, want command name", err)
	}
}
