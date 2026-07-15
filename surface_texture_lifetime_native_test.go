//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"testing"

	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu/core"
	"github.com/gogpu/wgpu/hal"
	"github.com/gogpu/wgpu/hal/noop"
)

type surfaceTextureLifetimeDevice struct {
	noop.Device
	destroyedTextures int
	destroyedViews    int
}

func (d *surfaceTextureLifetimeDevice) DestroyTexture(texture hal.Texture) {
	d.destroyedTextures++
	d.Device.DestroyTexture(texture)
}

func (d *surfaceTextureLifetimeDevice) DestroyTextureView(view hal.TextureView) {
	d.destroyedViews++
	d.Device.DestroyTextureView(view)
}

func newAcquiredSurfaceForLifetimeTest(t *testing.T) (*Surface, *SurfaceTexture, *Device, *surfaceTextureLifetimeDevice) {
	t.Helper()

	rawDevice := &surfaceTextureLifetimeDevice{}
	coreDevice := core.NewDevice(rawDevice, nil, 0, gputypes.DefaultLimits(), "surface-texture-lifetime-test")
	queue := &Queue{hal: &noop.Queue{}, halDevice: rawDevice}
	device := &Device{core: coreDevice, queue: queue}
	queue.device = device

	rawSurface := &noop.Surface{}
	surface := &Surface{
		core:           core.NewSurface(rawSurface, "surface-texture-lifetime-test"),
		device:         device,
		surfaceCreated: true,
		currentBackend: gputypes.BackendEmpty,
	}
	config := &SurfaceConfiguration{
		Width:       1,
		Height:      1,
		Format:      gputypes.TextureFormatRGBA8Unorm,
		Usage:       gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopyDst,
		PresentMode: gputypes.PresentModeFifo,
		AlphaMode:   gputypes.CompositeAlphaModeAuto,
	}
	if err := surface.Configure(device, config); err != nil {
		device.Release()
		t.Fatalf("surface.Configure: %v", err)
	}
	texture, _, err := surface.GetCurrentTexture()
	if err != nil {
		surface.Release()
		device.Release()
		t.Fatalf("surface.GetCurrentTexture: %v", err)
	}
	return surface, texture, device, rawDevice
}

func TestSurfaceTextureDerivedWrappersInvalidateAfterDiscard(t *testing.T) {
	surface, surfaceTexture, device, rawDevice := newAcquiredSurfaceForLifetimeTest(t)
	defer device.Release()

	texture := surfaceTexture.AsTexture()
	if texture == nil {
		t.Fatal("AsTexture returned nil for acquired texture")
	}
	texture.Release()
	if rawDevice.destroyedTextures != 0 {
		t.Fatalf("borrowed surface texture destruction count = %d, want 0", rawDevice.destroyedTextures)
	}
	view, err := surfaceTexture.CreateView(nil)
	if err != nil {
		t.Fatalf("CreateView: %v", err)
	}
	texture = surfaceTexture.AsTexture()
	if texture == nil {
		t.Fatal("second AsTexture returned nil while acquisition was active")
	}

	surface.DiscardTexture()
	if got := surfaceTexture.AsTexture(); got != nil {
		t.Fatal("AsTexture returned a wrapper after DiscardTexture")
	}
	if _, err := surfaceTexture.CreateView(nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("CreateView after DiscardTexture = %v, want ErrReleased", err)
	}
	if _, err := device.CreateTextureView(texture, nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("Device.CreateTextureView after DiscardTexture = %v, want ErrReleased", err)
	}
	if err := device.Queue().WriteTexture(&ImageCopyTexture{Texture: texture}, nil, nil, nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("Queue.WriteTexture after DiscardTexture = %v, want ErrReleased", err)
	}

	view.Release()
	if rawDevice.destroyedViews != 0 {
		t.Fatalf("stale surface view destruction count = %d, want 0", rawDevice.destroyedViews)
	}
	if view.Texture() != nil {
		t.Fatal("TextureView.Texture returned parent after stale view release")
	}
	surface.Release()
}

func TestSurfaceTextureDerivedWrappersInvalidateAfterPresentAndRelease(t *testing.T) {
	surface, surfaceTexture, device, rawDevice := newAcquiredSurfaceForLifetimeTest(t)
	defer device.Release()

	texture := surfaceTexture.AsTexture()
	view, err := surfaceTexture.CreateView(nil)
	if err != nil {
		t.Fatalf("CreateView: %v", err)
	}
	if err := surface.Present(surfaceTexture); err != nil {
		t.Fatalf("surface.Present: %v", err)
	}
	if _, err := device.CreateTextureView(texture, nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("Device.CreateTextureView after Present = %v, want ErrReleased", err)
	}
	view.Release()
	if rawDevice.destroyedViews != 0 {
		t.Fatalf("stale presented view destruction count = %d, want 0", rawDevice.destroyedViews)
	}

	// A later acquisition is invalidated by surface teardown as well.
	surfaceTexture, _, err = surface.GetCurrentTexture()
	if err != nil {
		t.Fatalf("second GetCurrentTexture: %v", err)
	}
	texture = surfaceTexture.AsTexture()
	view, err = surfaceTexture.CreateView(nil)
	if err != nil {
		t.Fatalf("second CreateView: %v", err)
	}
	surface.Release()
	if surfaceTexture.AsTexture() != nil {
		t.Fatal("AsTexture returned a wrapper after Surface.Release")
	}
	if _, err := device.CreateTextureView(texture, nil); !errors.Is(err, ErrReleased) {
		t.Fatalf("Device.CreateTextureView after Surface.Release = %v, want ErrReleased", err)
	}
	view.Release()
	if rawDevice.destroyedViews != 0 {
		t.Fatalf("stale released-surface view destruction count = %d, want 0", rawDevice.destroyedViews)
	}
}

func TestSurfacePresentRejectsTextureFromAnotherAcquisition(t *testing.T) {
	surface, surfaceTexture, device, _ := newAcquiredSurfaceForLifetimeTest(t)
	defer device.Release()

	other := &SurfaceTexture{hal: surfaceTexture.hal, surface: surface, device: device, token: newSurfaceTextureToken()}
	if err := surface.Present(other); !errors.Is(err, ErrReleased) {
		t.Fatalf("Present with foreign token = %v, want ErrReleased", err)
	}
	if surface.core.State() != core.SurfaceStateAcquired {
		t.Fatalf("surface state after rejected Present = %v, want acquired", surface.core.State())
	}
	surface.DiscardTexture()
	surface.Release()
}
