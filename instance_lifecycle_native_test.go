//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"slices"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal/noop"
)

type instanceLifecycleDevice struct {
	noop.Device
	events *[]string
}

func (d *instanceLifecycleDevice) WaitIdle() error {
	*d.events = append(*d.events, "device-wait-idle")
	return nil
}

func (d *instanceLifecycleDevice) Destroy() {
	*d.events = append(*d.events, "device-destroy")
}

type instanceLifecycleSurface struct {
	noop.Surface
	events *[]string
}

func (s *instanceLifecycleSurface) Destroy() {
	*s.events = append(*s.events, "surface-destroy")
}

func TestInstanceReleaseOwnsNativeSurfaceAndDeviceOrder(t *testing.T) {
	events := []string{}
	instance := &Instance{core: core.NewInstanceWithMock(nil)}
	rawDevice := &instanceLifecycleDevice{events: &events}
	queue := &Queue{hal: &noop.Queue{}, halDevice: rawDevice}
	device := &Device{
		core:  core.NewDevice(rawDevice, nil, 0, gputypes.DefaultLimits(), "lifecycle-test"),
		queue: queue,
	}
	queue.device = device
	if err := instance.adoptDevice(device); err != nil {
		t.Fatalf("adopt device: %v", err)
	}
	rawSurface := &instanceLifecycleSurface{events: &events}
	surface := &Surface{
		core:     core.NewSurface(rawSurface, "lifecycle-test"),
		instance: instance,
	}
	if err := instance.adoptSurface(surface); err != nil {
		t.Fatalf("adopt surface: %v", err)
	}

	instance.Release()
	want := []string{"surface-destroy", "device-wait-idle", "device-destroy"}
	if !slices.Equal(events, want) {
		t.Fatalf("release events = %v, want %v", events, want)
	}

	// Retained public wrappers must not call into already-destroyed HAL objects.
	adapter := &Adapter{instance: instance}
	if capabilities := adapter.GetSurfaceCapabilities(surface); capabilities != nil {
		t.Fatalf("retained adapter returned capabilities after instance release: %+v", capabilities)
	}
	surface.SetPrepareFrame(nil)
	if raw := surface.HAL(); raw != nil {
		t.Fatalf("retained surface returned HAL object after instance release: %T", raw)
	}
	if _, err := queue.Submit(); !errors.Is(err, ErrReleased) {
		t.Fatalf("retained queue Submit after instance release = %v, want ErrReleased", err)
	}
	if completed := queue.Poll(); completed != 0 {
		t.Fatalf("retained queue Poll after instance release = %d, want 0", completed)
	}
	queue.SetSwapchainSuppressed(true)
	surface.Release()
	device.Release()
	if !slices.Equal(events, want) {
		t.Fatalf("post-release wrapper calls changed events to %v", events)
	}
}
