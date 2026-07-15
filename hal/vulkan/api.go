//go:build !(js && wasm)

// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/vulkan/vk"
)

// Backend implements hal.Backend for Vulkan.
type Backend struct{}

// Variant returns the backend type identifier.
func (Backend) Variant() gputypes.Backend {
	return gputypes.BackendVulkan
}

// CreateInstance creates a new Vulkan instance.
func (Backend) CreateInstance(desc *hal.InstanceDescriptor) (hal.Instance, error) {
	platform, err := newPlatformInstanceState(desc)
	if err != nil {
		return nil, err
	}

	// Initialize Vulkan library
	if err := vk.Init(); err != nil {
		return nil, fmt.Errorf("vulkan: failed to initialize: %w", err)
	}

	// Create Commands and load global Vulkan functions
	cmds := vk.NewCommands()
	if err := cmds.LoadGlobal(); err != nil {
		return nil, fmt.Errorf("vulkan: failed to load global commands: %w", err)
	}

	loaderVersion, err := requireVulkan12(cmds)
	if err != nil {
		return nil, err
	}
	availableExtensions, err := enumerateInstanceExtensions(cmds)
	if err != nil {
		return nil, fmt.Errorf("vulkan: enumerate instance extensions: %w", err)
	}

	// Prepare application info
	appName := []byte("gogpu\x00")
	engineName := []byte("gogpu/wgpu\x00")

	appInfo := vk.ApplicationInfo{
		SType:              vk.StructureTypeApplicationInfo,
		PApplicationName:   uintptr(unsafe.Pointer(&appName[0])),
		ApplicationVersion: vkMakeVersion(1, 0, 0),
		PEngineName:        uintptr(unsafe.Pointer(&engineName[0])),
		EngineVersion:      vkMakeVersion(0, 1, 0),
		ApiVersion:         vkMakeVersion(1, 2, 0),
	}

	// Optional: validation layers for debug (only if available)
	var layers []string
	var validationEnabled bool
	if desc != nil && desc.Flags&gputypes.InstanceFlagsDebug != 0 {
		_, debugUtilsAvailable := availableExtensions["VK_EXT_debug_utils"]
		if debugUtilsAvailable && isLayerAvailable(cmds, "VK_LAYER_KHRONOS_validation") {
			layers = append(layers, "VK_LAYER_KHRONOS_validation\x00")
			validationEnabled = true
		}
		// Silently skip if validation layers not installed (Vulkan SDK not present)
	}

	extensions, surfaceEnabled := selectInstanceExtensions(
		availableExtensions,
		platformSurfaceExtension(),
		validationEnabled,
	)

	// Convert to C strings
	extensionPtrs := make([]uintptr, len(extensions))
	for i, ext := range extensions {
		extensionPtrs[i] = uintptr(unsafe.Pointer(unsafe.StringData(ext)))
	}

	layerPtrs := make([]uintptr, len(layers))
	for i, layer := range layers {
		layerPtrs[i] = uintptr(unsafe.Pointer(unsafe.StringData(layer)))
	}

	// Create instance
	createInfo := vk.InstanceCreateInfo{
		SType:                 vk.StructureTypeInstanceCreateInfo,
		PApplicationInfo:      &appInfo,
		EnabledExtensionCount: uint32(len(extensions)),
		EnabledLayerCount:     uint32(len(layers)),
	}

	if len(extensionPtrs) > 0 {
		createInfo.PpEnabledExtensionNames = uintptr(unsafe.Pointer(&extensionPtrs[0]))
	}
	if len(layerPtrs) > 0 {
		createInfo.PpEnabledLayerNames = uintptr(unsafe.Pointer(&layerPtrs[0]))
	}

	var instance vk.Instance
	result := cmds.CreateInstance(&createInfo, nil, &instance)
	if result != vk.Success {
		return nil, mapVulkanResult("vkCreateInstance", result)
	}

	// Load instance-level commands
	if err := cmds.LoadInstance(instance); err != nil {
		cmds.DestroyInstance(instance, nil)
		return nil, fmt.Errorf("vulkan: failed to load instance commands: %w", err)
	}

	// Set vkGetDeviceProcAddr for device function loading.
	// Some drivers (e.g., Intel) don't support loading it with instance=0.
	vk.SetDeviceProcAddr(instance)
	if surfaceEnabled && !cmds.HasWSIQueries() {
		// Keep the instance usable for headless workloads. CreateSurface will
		// fail closed instead of making instance creation depend on WSI.
		surfaceEnabled = false
	}

	// Keep references alive
	runtime.KeepAlive(appName)
	runtime.KeepAlive(engineName)
	runtime.KeepAlive(extensions)
	runtime.KeepAlive(layers)
	runtime.KeepAlive(extensionPtrs)
	runtime.KeepAlive(layerPtrs)

	inst := &Instance{
		handle:         instance,
		cmds:           *cmds,
		debugEnabled:   validationEnabled,
		surfaceEnabled: surfaceEnabled,
		platform:       platform,
	}

	// Create debug messenger when validation layers are active.
	// This captures validation errors and logs them via Go's log package.
	if validationEnabled {
		inst.debugMessenger = createDebugMessenger(inst)
	}

	hal.Logger().Info("vulkan: instance created",
		"apiVersion", fmt.Sprintf("%d.%d.%d", vkVersionMajor(appInfo.ApiVersion), vkVersionMinor(appInfo.ApiVersion), vkVersionPatch(appInfo.ApiVersion)),
		"loaderVersion", fmt.Sprintf("%d.%d.%d", vkVersionMajor(loaderVersion), vkVersionMinor(loaderVersion), vkVersionPatch(loaderVersion)),
		"validation", validationEnabled,
	)

	return inst, nil
}

// Instance implements hal.Instance for Vulkan.
type Instance struct {
	handle         vk.Instance
	cmds           vk.Commands
	debugMessenger vk.DebugUtilsMessengerEXT
	debugEnabled   bool
	surfaceEnabled bool
	platform       platformInstanceState

	creationMu  sync.RWMutex
	lifecycleMu sync.Mutex
	destroying  bool
	devices     map[*Device]struct{}
	surfaces    map[*Surface]struct{}
}

func (i *Instance) beginResourceCreation() error {
	if i == nil {
		return fmt.Errorf("vulkan: instance is nil")
	}
	i.creationMu.RLock()
	i.lifecycleMu.Lock()
	unavailable := i.destroying || i.handle == 0
	i.lifecycleMu.Unlock()
	if unavailable {
		i.creationMu.RUnlock()
		return fmt.Errorf("vulkan: instance is being destroyed")
	}
	return nil
}

func (i *Instance) endResourceCreation() {
	i.creationMu.RUnlock()
}

func (i *Instance) registerDevice(device *Device) error {
	if i == nil || device == nil {
		return fmt.Errorf("vulkan: cannot register a nil device")
	}
	i.lifecycleMu.Lock()
	defer i.lifecycleMu.Unlock()
	if i.destroying || i.handle == 0 {
		return fmt.Errorf("vulkan: instance is being destroyed")
	}
	if i.devices == nil {
		i.devices = make(map[*Device]struct{})
	}
	i.devices[device] = struct{}{}
	return nil
}

func (i *Instance) unregisterDevice(device *Device) {
	if i == nil || device == nil {
		return
	}
	i.lifecycleMu.Lock()
	delete(i.devices, device)
	i.lifecycleMu.Unlock()
}

func (i *Instance) registerSurface(surface *Surface) error {
	if i == nil || surface == nil {
		return fmt.Errorf("vulkan: cannot register a nil surface")
	}
	i.lifecycleMu.Lock()
	defer i.lifecycleMu.Unlock()
	if i.destroying || i.handle == 0 {
		return fmt.Errorf("vulkan: instance is being destroyed")
	}
	if i.surfaces == nil {
		i.surfaces = make(map[*Surface]struct{})
	}
	i.surfaces[surface] = struct{}{}
	return nil
}

func (i *Instance) unregisterSurface(surface *Surface) {
	if i == nil || surface == nil {
		return
	}
	i.lifecycleMu.Lock()
	delete(i.surfaces, surface)
	i.lifecycleMu.Unlock()
}

func (i *Instance) adoptSurface(surface *Surface) (hal.Surface, error) {
	if surface == nil || surface.handle == 0 {
		return nil, fmt.Errorf("vulkan: cannot adopt a null surface")
	}
	if err := i.registerSurface(surface); err != nil {
		i.cmds.DestroySurfaceKHR(i.handle, surface.handle, nil)
		surface.handle = 0
		return nil, fmt.Errorf("vulkan: register surface lifetime: %w", err)
	}
	return surface, nil
}

func (i *Instance) beginDestroyResources() ([]*Surface, []*Device, bool) {
	if i == nil {
		return nil, nil, false
	}
	i.lifecycleMu.Lock()
	defer i.lifecycleMu.Unlock()
	if i.destroying || i.handle == 0 {
		return nil, nil, false
	}
	i.destroying = true
	surfaces := make([]*Surface, 0, len(i.surfaces))
	for surface := range i.surfaces {
		surfaces = append(surfaces, surface)
	}
	devices := make([]*Device, 0, len(i.devices))
	for device := range i.devices {
		devices = append(devices, device)
	}
	clear(i.surfaces)
	clear(i.devices)
	return surfaces, devices, true
}

// EnumerateAdapters returns available Vulkan adapters (physical devices).
func (i *Instance) EnumerateAdapters(surfaceHint hal.Surface) []hal.ExposedAdapter {
	devices, err := enumeratePhysicalDevicesWith(func(count *uint32, devices *vk.PhysicalDevice) vk.Result {
		return i.cmds.EnumeratePhysicalDevices(i.handle, count, devices)
	})
	if err != nil {
		hal.Logger().Error("vulkan: failed to enumerate physical devices", "error", err)
		return nil
	}
	if len(devices) == 0 {
		return nil
	}

	adapters := make([]hal.ExposedAdapter, 0, len(devices))
	hal.Logger().Debug("vulkan: enumerating adapters", "count", len(devices))
	for _, device := range devices {
		// Get device properties
		var props vk.PhysicalDeviceProperties
		i.cmds.GetPhysicalDeviceProperties(device, &props)
		if props.ApiVersion < vkMakeVersion(1, 2, 0) {
			hal.Logger().Debug("vulkan: adapter below Vulkan 1.2 floor",
				"apiVersion", fmt.Sprintf("%d.%d.%d", vkVersionMajor(props.ApiVersion), vkVersionMinor(props.ApiVersion), vkVersionPatch(props.ApiVersion)),
			)
			continue
		}

		// Get device features
		var features vk.PhysicalDeviceFeatures
		i.cmds.GetPhysicalDeviceFeatures(device, &features)

		// Convert device type
		deviceType := gputypes.DeviceTypeOther
		switch props.DeviceType {
		case vk.PhysicalDeviceTypeDiscreteGpu:
			deviceType = gputypes.DeviceTypeDiscreteGPU
		case vk.PhysicalDeviceTypeIntegratedGpu:
			deviceType = gputypes.DeviceTypeIntegratedGPU
		case vk.PhysicalDeviceTypeVirtualGpu:
			deviceType = gputypes.DeviceTypeVirtualGPU
		case vk.PhysicalDeviceTypeCpu:
			deviceType = gputypes.DeviceTypeCPU
		}

		// Extract device name
		deviceName := cStringToGo(props.DeviceName[:])

		adapter := &Adapter{
			instance:       i,
			physicalDevice: device,
			properties:     props,
			features:       features,
		}

		adapterForExpose := hal.Adapter(adapter)
		if surfaceHint != nil {
			qualified, err := adapter.QualifySurface(surfaceHint)
			if err != nil {
				hal.Logger().Debug("vulkan: adapter rejected surface hint", "name", deviceName, "error", err)
				continue
			}
			adapterForExpose = qualified
		}

		hal.Logger().Info("vulkan: adapter found",
			"name", deviceName,
			"type", deviceType,
			"vendor", vendorIDToName(props.VendorID),
			"apiVersion", fmt.Sprintf("%d.%d.%d", vkVersionMajor(props.ApiVersion), vkVersionMinor(props.ApiVersion), vkVersionPatch(props.ApiVersion)),
		)

		adapters = append(adapters, hal.ExposedAdapter{
			Adapter: adapterForExpose,
			Info: gputypes.AdapterInfo{
				Name:       deviceName,
				Vendor:     vendorIDToName(props.VendorID),
				VendorID:   props.VendorID,
				DeviceID:   props.DeviceID,
				DeviceType: deviceType,
				Driver:     "Vulkan",
				DriverInfo: fmt.Sprintf("Vulkan %d.%d.%d",
					vkVersionMajor(props.ApiVersion),
					vkVersionMinor(props.ApiVersion),
					vkVersionPatch(props.ApiVersion)),
				Backend: gputypes.BackendVulkan,
			},
			Features: featuresFromPhysicalDevice(&features),
			Capabilities: hal.Capabilities{
				Limits: limitsFromProps(&props),
				AlignmentsMask: hal.Alignments{
					BufferCopyOffset: 4,
					BufferCopyPitch:  256,
				},
				DownlevelCapabilities: hal.DownlevelCapabilities{
					ShaderModel: 60, // SM6.0 equivalent
					Flags:       0,
				},
			},
		})
	}

	return adapters
}

func enumeratePhysicalDevicesWith(query func(count *uint32, devices *vk.PhysicalDevice) vk.Result) ([]vk.PhysicalDevice, error) {
	const attempts = 3
	for attempt := 0; attempt < attempts; attempt++ {
		var count uint32
		result := query(&count, nil)
		if result != vk.Success && result != vk.Incomplete {
			return nil, fmt.Errorf("vulkan: physical device count: %w", mapVulkanResult("vkEnumeratePhysicalDevices", result))
		}
		if count == 0 {
			if result == vk.Incomplete {
				continue
			}
			return nil, nil
		}
		devices := make([]vk.PhysicalDevice, count)
		returned := count
		result = query(&returned, &devices[0])
		if result != vk.Success && result != vk.Incomplete {
			return nil, mapVulkanResult("vkEnumeratePhysicalDevices", result)
		}
		if result == vk.Incomplete || returned > uint32(len(devices)) {
			continue
		}
		devices = devices[:returned]
		for _, device := range devices {
			if device == 0 {
				return nil, fmt.Errorf("vulkan: vkEnumeratePhysicalDevices returned a null device")
			}
		}
		return devices, nil
	}
	return nil, fmt.Errorf("vulkan: vkEnumeratePhysicalDevices returned an unstable count after %d attempts", attempts)
}

// Destroy releases the Vulkan instance.
func (i *Instance) Destroy() {
	if i == nil {
		return
	}
	// Native surface/device creation holds the read side until the new object is
	// registered. Destruction cannot pass it and invalidate VkInstance in the
	// create-before-adopt window.
	i.creationMu.Lock()
	defer i.creationMu.Unlock()
	surfaces, devices, ok := i.beginDestroyResources()
	if !ok {
		return
	}

	// Vulkan requires every surface/swapchain and logical device to be gone
	// before VkInstance. Retain those relationships explicitly, matching Rust
	// wgpu's shared-instance ownership even when callers release out of order.
	for _, surface := range surfaces {
		surface.Destroy()
	}
	for _, device := range devices {
		device.Destroy()
	}

	// Destroy debug messenger before the instance (required by Vulkan spec).
	if i.debugMessenger != 0 {
		destroyDebugMessenger(i, i.debugMessenger)
		i.debugMessenger = 0
	}
	i.cmds.DestroyInstance(i.handle, nil)
	i.handle = 0
}

// Surface implements hal.Surface for Vulkan.
type Surface struct {
	mu               sync.Mutex
	handle           vk.SurfaceKHR
	instance         *Instance
	swapchain        *Swapchain
	device           *Device
	destroyRequested bool
	platform         platformSurfaceState
}

// Configure configures the surface for presentation.
//
// Returns hal.ErrZeroArea if width or height is zero.
// This commonly happens when the window is minimized or not yet fully visible.
// Wait until the window has valid dimensions before calling Configure again.
func (s *Surface) Configure(device hal.Device, config *hal.SurfaceConfiguration) error {
	if s == nil {
		return fmt.Errorf("vulkan: surface is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.destroyRequested || s.handle == 0 {
		return hal.ErrSurfaceLost
	}
	if config == nil {
		return fmt.Errorf("vulkan: surface configuration is nil")
	}
	if err := s.validatePlatform(); err != nil {
		return err
	}
	// Validate dimensions first (before any side effects).
	// This matches wgpu-core behavior which returns ConfigureSurfaceError::ZeroArea.
	if config.Width == 0 || config.Height == 0 {
		return hal.ErrZeroArea
	}

	vkDevice, ok := device.(*Device)
	if !ok {
		return fmt.Errorf("vulkan: device is not a Vulkan device")
	}
	hal.Logger().Info("vulkan: surface configuring",
		"width", config.Width,
		"height", config.Height,
		"format", config.Format,
		"presentMode", config.PresentMode,
	)
	return s.createSwapchain(vkDevice, config)
}

// ActualExtent returns the actual swapchain dimensions after driver clamping.
// On Vulkan, the driver may clamp the requested extent to its supported range
// (e.g., on X11 HiDPI). Returns (0, 0) if no swapchain is configured.
func (s *Surface) ActualExtent() (width, height uint32) {
	if s == nil {
		return 0, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.swapchain == nil {
		return 0, 0
	}
	return s.swapchain.extent.Width, s.swapchain.extent.Height
}

// Unconfigure removes surface configuration.
func (s *Surface) Unconfigure(_ hal.Device) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.swapchain != nil {
		swapchain := s.swapchain
		if err := swapchain.destroyWithError(); err != nil {
			hal.Logger().Error("vulkan: failed to destroy swapchain during unconfigure", "error", err)
			return
		}
		s.clearActiveSwapchain(swapchain)
		s.swapchain = nil
	}
	if s.device != nil {
		s.device.unregisterConfiguredSurface(s)
	}
	s.device = nil
}

// AcquireTexture acquires the next surface texture for rendering.
// Returns hal.ErrNotReady if no image is available (non-blocking mode).
func (s *Surface) AcquireTexture(_ hal.Fence) (*hal.AcquiredSurfaceTexture, error) {
	if s == nil {
		return nil, fmt.Errorf("vulkan: surface is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.destroyRequested || s.handle == 0 {
		return nil, hal.ErrSurfaceLost
	}
	if err := s.validatePlatform(); err != nil {
		return nil, err
	}
	if s.swapchain == nil {
		return nil, fmt.Errorf("vulkan: surface not configured")
	}

	texture, suboptimal, err := s.swapchain.acquireNextImage()
	if err != nil {
		return nil, err
	}

	// No image available right now - skip this frame
	if texture == nil {
		return nil, hal.ErrNotReady
	}

	// Register swapchain with queue for proper synchronization in Submit.
	// This ensures the queue waits for image acquisition before rendering
	// and signals completion before present.
	if s.device != nil && s.device.queue != nil {
		s.device.queue.mu.Lock()
		s.device.queue.activeSwapchain = s.swapchain
		s.device.queue.acquireUsed = false // Reset for new frame
		s.device.queue.mu.Unlock()
	}

	return &hal.AcquiredSurfaceTexture{
		Texture:    texture,
		Suboptimal: suboptimal,
	}, nil
}

// DiscardTexture discards a surface texture without presenting it.
func (s *Surface) DiscardTexture(_ hal.SurfaceTexture) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.swapchain != nil && s.swapchain.imageAcquired {
		s.swapchain.markBroken(fmt.Errorf("vulkan: acquired surface texture was discarded without presentation"))
		if s.device != nil && s.device.queue != nil {
			s.device.queue.mu.Lock()
			if s.device.queue.activeSwapchain == s.swapchain {
				s.device.queue.activeSwapchain = nil
				s.device.queue.acquireUsed = false
			}
			s.device.queue.mu.Unlock()
		}
	}
}

// Destroy releases the surface.
func (s *Surface) Destroy() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.destroyRequested = true
	if s.swapchain != nil {
		swapchain := s.swapchain
		if err := swapchain.destroyWithError(); err != nil {
			hal.Logger().Error("vulkan: failed to destroy swapchain", "error", err)
			// Keep the surface registered. Device.Destroy will either drain the
			// swapchain or abandon it after device loss, then complete this pending
			// surface destruction without using a dead VkDevice.
			return
		}
		s.clearActiveSwapchain(swapchain)
		s.swapchain = nil
	}
	if s.device != nil {
		s.device.unregisterConfiguredSurface(s)
	}
	s.device = nil
	s.destroySurfaceHandleLocked()
}

func (s *Surface) clearActiveSwapchain(swapchain *Swapchain) {
	if s == nil || s.device == nil || s.device.queue == nil {
		return
	}
	s.device.queue.mu.Lock()
	if s.device.queue.activeSwapchain == swapchain {
		s.device.queue.activeSwapchain = nil
		s.device.queue.acquireUsed = false
	}
	s.device.queue.mu.Unlock()
}

func (s *Surface) destroySurfaceHandle() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.destroySurfaceHandleLocked()
}

func (s *Surface) destroySurfaceHandleLocked() {
	instance := s.instance
	if s.handle == 0 {
		if instance != nil {
			instance.unregisterSurface(s)
		}
		return
	}
	if s.instance == nil || s.instance.handle == 0 {
		hal.Logger().Error("vulkan: cannot destroy surface after its instance")
		return
	}
	s.instance.cmds.DestroySurfaceKHR(s.instance.handle, s.handle, nil)
	s.handle = 0
	instance.unregisterSurface(s)
}

// releaseConfiguredDevice is called only by Device.Destroy after its single
// device-wide idle attempt. It disconnects the swapchain before VkDevice is
// destroyed, while preserving the VkSurface until its instance is released.
func (s *Surface) releaseConfiguredDevice(device *Device, drained bool) bool {
	if s == nil || device == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.device != device {
		return false
	}
	swapchain := s.swapchain
	s.clearActiveSwapchain(swapchain)
	childrenReleased := releaseSwapchainFromDevice(swapchain, device, drained)
	s.swapchain = nil
	s.device = nil
	if s.destroyRequested && childrenReleased {
		s.destroySurfaceHandleLocked()
	}
	return s.destroyRequested && !childrenReleased
}

func releaseSwapchainFromDevice(swapchain *Swapchain, device *Device, drained bool) bool {
	if swapchain == nil {
		return true
	}
	if !drained {
		swapchain.abandonDeviceResources()
		return false
	}
	if err := swapchain.destroyResourcesAfterIdle(); err != nil {
		hal.Logger().Error("vulkan: failed to release drained swapchain resources", "error", err)
		swapchain.abandonDeviceResources()
		return false
	}
	if swapchain.handle != 0 {
		vkDestroySwapchainKHR(device, swapchain.handle, nil)
		swapchain.handle = 0
	}
	swapchain.destroyed = true
	swapchain.broken = true
	swapchain.device = nil
	swapchain.surface = nil
	return true
}

// Helper functions

func vkMakeVersion(major, minor, patch uint32) uint32 {
	return (major << 22) | (minor << 12) | patch
}

func vkVersionMajor(version uint32) uint32 {
	return version >> 22
}

func vkVersionMinor(version uint32) uint32 {
	return (version >> 12) & 0x3FF
}

func vkVersionPatch(version uint32) uint32 {
	return version & 0xFFF
}

func requireVulkan12(cmds *vk.Commands) (uint32, error) {
	if !cmds.HasEnumerateInstanceVersion() {
		return 0, fmt.Errorf("vulkan: loader does not expose Vulkan 1.2")
	}
	var version uint32
	if result := cmds.EnumerateInstanceVersion(&version); result != vk.Success {
		return 0, mapVulkanResult("vkEnumerateInstanceVersion", result)
	}
	if err := validateVulkanVersion(version); err != nil {
		return 0, err
	}
	return version, nil
}

func validateVulkanVersion(version uint32) error {
	if version < vkMakeVersion(1, 2, 0) {
		return fmt.Errorf("vulkan: Vulkan 1.2 is required, loader reports %d.%d.%d",
			vkVersionMajor(version), vkVersionMinor(version), vkVersionPatch(version))
	}
	return nil
}

func enumerateInstanceExtensions(cmds *vk.Commands) (map[string]struct{}, error) {
	return enumerateInstanceExtensionsWith(func(count *uint32, properties *vk.ExtensionProperties) vk.Result {
		return cmds.EnumerateInstanceExtensionProperties(0, count, properties)
	})
}

func enumerateInstanceExtensionsWith(query func(count *uint32, properties *vk.ExtensionProperties) vk.Result) (map[string]struct{}, error) {
	const attempts = 3
	for attempt := 0; attempt < attempts; attempt++ {
		var count uint32
		result := query(&count, nil)
		if result != vk.Success && result != vk.Incomplete {
			return nil, fmt.Errorf("count query: %w", mapVulkanResult("vkEnumerateInstanceExtensionProperties", result))
		}
		if count == 0 {
			if result == vk.Incomplete {
				continue
			}
			return map[string]struct{}{}, nil
		}
		properties := make([]vk.ExtensionProperties, count)
		returned := count
		result = query(&returned, &properties[0])
		if result == vk.Incomplete || returned > uint32(len(properties)) {
			continue
		}
		if result != vk.Success {
			return nil, fmt.Errorf("property query: %w", mapVulkanResult("vkEnumerateInstanceExtensionProperties", result))
		}
		available := make(map[string]struct{}, returned)
		for index := range properties[:returned] {
			available[cStringToGo(properties[index].ExtensionName[:])] = struct{}{}
		}
		return available, nil
	}
	return nil, fmt.Errorf("vulkan: vkEnumerateInstanceExtensionProperties returned an unstable count after %d attempts", attempts)
}

func selectInstanceExtensions(available map[string]struct{}, platformExtension string, debug bool) (extensions []string, surfaceEnabled bool) {
	platformExtension = strings.TrimSuffix(platformExtension, "\x00")
	_, hasSurface := available["VK_KHR_surface"]
	_, hasPlatform := available[platformExtension]
	if hasSurface {
		extensions = append(extensions, "VK_KHR_surface\x00")
	}
	if hasSurface && hasPlatform {
		extensions = append(extensions, platformExtension+"\x00")
	}
	if debug {
		if _, ok := available["VK_EXT_debug_utils"]; ok {
			extensions = append(extensions, "VK_EXT_debug_utils\x00")
		}
	}
	return extensions, hasSurface && hasPlatform
}

func cStringToGo(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// GPU vendor names used in vendorIDToName and adapter info.
const (
	vendorAMD      = "AMD"
	vendorNVIDIA   = "NVIDIA"
	vendorIntel    = "Intel"
	vendorARM      = "ARM"
	vendorQualcomm = "Qualcomm"
	vendorImgTec   = "ImgTec"
)

func vendorIDToName(id uint32) string {
	switch id {
	case 0x1002:
		return vendorAMD
	case 0x10DE:
		return vendorNVIDIA
	case 0x8086:
		return vendorIntel
	case 0x13B5:
		return vendorARM
	case 0x5143:
		return vendorQualcomm
	case 0x1010:
		return vendorImgTec
	default:
		return fmt.Sprintf("0x%04X", id)
	}
}

// featuresFromPhysicalDevice maps Vulkan physical device features to WebGPU features.
// Reference: wgpu-hal/src/vulkan/adapter.rs:584-829
func featuresFromPhysicalDevice(features *vk.PhysicalDeviceFeatures) gputypes.Features {
	var result gputypes.Features

	// Texture compression features
	if features.TextureCompressionBC != 0 {
		result |= gputypes.Features(gputypes.FeatureTextureCompressionBC)
	}
	if features.TextureCompressionETC2 != 0 {
		result |= gputypes.Features(gputypes.FeatureTextureCompressionETC2)
	}
	if features.TextureCompressionASTC_LDR != 0 {
		result |= gputypes.Features(gputypes.FeatureTextureCompressionASTC)
	}

	// Draw features
	if features.DrawIndirectFirstInstance != 0 {
		result |= gputypes.Features(gputypes.FeatureIndirectFirstInstance)
	}
	if features.MultiDrawIndirect != 0 {
		result |= gputypes.Features(gputypes.FeatureMultiDrawIndirect)
	}

	// Depth/clipping features
	if features.DepthClamp != 0 {
		result |= gputypes.Features(gputypes.FeatureDepthClipControl)
	}

	// Shader features
	if features.ShaderFloat64 != 0 {
		result |= gputypes.Features(gputypes.FeatureShaderFloat64)
	}

	// Query features
	if features.PipelineStatisticsQuery != 0 {
		result |= gputypes.Features(gputypes.FeaturePipelineStatisticsQuery)
	}

	// Depth32FloatStencil8 is always available in Vulkan 1.0+
	result |= gputypes.Features(gputypes.FeatureDepth32FloatStencil8)

	return result
}

// limitsFromProps maps Vulkan physical device limits to WebGPU limits.
// Reference: wgpu-hal/src/vulkan/adapter.rs:1254-1392
func limitsFromProps(props *vk.PhysicalDeviceProperties) gputypes.Limits {
	vkLimits := props.Limits

	// Start with default limits and override with actual hardware values
	limits := gputypes.DefaultLimits()

	// Texture dimensions
	limits.MaxTextureDimension1D = vkLimits.MaxImageDimension1D
	limits.MaxTextureDimension2D = vkLimits.MaxImageDimension2D
	limits.MaxTextureDimension3D = vkLimits.MaxImageDimension3D
	limits.MaxTextureArrayLayers = vkLimits.MaxImageArrayLayers

	// Descriptor/binding limits
	limits.MaxBindGroups = min(vkLimits.MaxBoundDescriptorSets, 8) // WebGPU max is 8
	limits.MaxSampledTexturesPerShaderStage = vkLimits.MaxPerStageDescriptorSampledImages
	limits.MaxSamplersPerShaderStage = vkLimits.MaxPerStageDescriptorSamplers
	limits.MaxStorageBuffersPerShaderStage = vkLimits.MaxPerStageDescriptorStorageBuffers
	limits.MaxStorageTexturesPerShaderStage = vkLimits.MaxPerStageDescriptorStorageImages
	limits.MaxUniformBuffersPerShaderStage = vkLimits.MaxPerStageDescriptorUniformBuffers

	// Buffer limits
	limits.MaxUniformBufferBindingSize = uint64(vkLimits.MaxUniformBufferRange)
	limits.MaxStorageBufferBindingSize = uint64(vkLimits.MaxStorageBufferRange)
	limits.MinUniformBufferOffsetAlignment = uint32(vkLimits.MinUniformBufferOffsetAlignment)
	limits.MinStorageBufferOffsetAlignment = uint32(vkLimits.MinStorageBufferOffsetAlignment)

	// Vertex limits
	limits.MaxVertexAttributes = min(vkLimits.MaxVertexInputAttributes, 32) // WebGPU max is 32
	limits.MaxVertexBufferArrayStride = min(vkLimits.MaxVertexInputBindingStride, 2048)

	// Color attachment limits
	limits.MaxColorAttachments = min(vkLimits.MaxColorAttachments, 8) // WebGPU max is 8

	// Compute limits
	limits.MaxComputeWorkgroupStorageSize = vkLimits.MaxComputeSharedMemorySize
	limits.MaxComputeInvocationsPerWorkgroup = vkLimits.MaxComputeWorkGroupInvocations
	limits.MaxComputeWorkgroupSizeX = vkLimits.MaxComputeWorkGroupSize[0]
	limits.MaxComputeWorkgroupSizeY = vkLimits.MaxComputeWorkGroupSize[1]
	limits.MaxComputeWorkgroupSizeZ = vkLimits.MaxComputeWorkGroupSize[2]
	limits.MaxComputeWorkgroupsPerDimension = vkLimits.MaxComputeWorkGroupCount[0]

	// Push constants
	limits.MaxPushConstantSize = vkLimits.MaxPushConstantsSize

	return limits
}

// isLayerAvailable checks if a Vulkan instance layer is available.
// Used to gracefully skip validation layers when Vulkan SDK is not installed.
func isLayerAvailable(cmds *vk.Commands, layerName string) bool {
	layers, err := enumerateInstanceLayersWith(func(count *uint32, layers *vk.LayerProperties) vk.Result {
		return cmds.EnumerateInstanceLayerProperties(count, layers)
	})
	if err != nil {
		hal.Logger().Warn("vulkan: validation layer enumeration failed", "error", err)
		return false
	}
	for i := range layers {
		name := cStringToGo(layers[i].LayerName[:])
		if name == layerName {
			return true
		}
	}
	return false
}

func enumerateInstanceLayersWith(query func(count *uint32, layers *vk.LayerProperties) vk.Result) ([]vk.LayerProperties, error) {
	const attempts = 3
	for attempt := 0; attempt < attempts; attempt++ {
		var count uint32
		result := query(&count, nil)
		if result != vk.Success && result != vk.Incomplete {
			return nil, fmt.Errorf("vulkan: instance layer count: %w", mapVulkanResult("vkEnumerateInstanceLayerProperties", result))
		}
		if count == 0 {
			if result == vk.Incomplete {
				continue
			}
			return nil, nil
		}
		layers := make([]vk.LayerProperties, count)
		returned := count
		result = query(&returned, &layers[0])
		if result != vk.Success && result != vk.Incomplete {
			return nil, mapVulkanResult("vkEnumerateInstanceLayerProperties", result)
		}
		if result == vk.Incomplete || returned > uint32(len(layers)) {
			continue
		}
		return layers[:returned], nil
	}
	return nil, fmt.Errorf("vulkan: vkEnumerateInstanceLayerProperties returned an unstable count after %d attempts", attempts)
}
