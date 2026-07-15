//go:build !rust && !(js && wasm)

package wgpu

import (
	"github.com/gogpu/wgpu/hal"
)

// Texture represents a GPU texture.
type Texture struct {
	hal          hal.Texture
	device       *Device
	format       TextureFormat
	released     bool
	surfaceToken *surfaceTextureToken
}

// surfaceTextureValid reports whether a texture derived from a surface
// acquisition may still be passed to native operations. Ordinary textures do
// not carry a token and remain governed by their released flag.
func (t *Texture) surfaceTextureValid() bool {
	if t == nil || t.released {
		return false
	}
	if t.surfaceToken == nil {
		return true
	}
	return t.surfaceToken.isValid() && t.device != nil && !t.device.released.Load()
}

// Format returns the texture format.
func (t *Texture) Format() TextureFormat { return t.format }

// Release destroys the texture. The underlying HAL texture is not freed
// immediately — destruction is deferred until the GPU completes any submission
// that may reference it. This prevents use-after-free on DX12/Vulkan.
func (t *Texture) Release() {
	if t.released {
		return
	}
	// Surface textures are borrowed swapchain images. The wrapper never owns
	// their HAL lifetime, even while the acquisition is still active.
	if t.surfaceToken != nil {
		t.released = true
		return
	}
	t.released = true

	halDevice := t.device.halDevice()
	if halDevice == nil {
		return
	}

	dq := t.device.destroyQueue()
	if dq == nil {
		halDevice.DestroyTexture(t.hal)
		return
	}

	subIdx := t.device.lastSubmissionIndex()
	halTex := t.hal
	dq.Defer(subIdx, "Texture", func() {
		halDevice.DestroyTexture(halTex)
	})
}

// TextureView represents a view into a texture.
type TextureView struct {
	hal          hal.TextureView
	device       *Device
	texture      *Texture
	released     bool
	surfaceToken *surfaceTextureToken
}

func (v *TextureView) surfaceTextureValid() bool {
	if v == nil || v.released {
		return false
	}
	if v.surfaceToken != nil && (!v.surfaceToken.isValid() || v.device == nil || v.device.released.Load()) {
		return false
	}
	return v.texture == nil || v.texture.surfaceTextureValid()
}

// Texture returns the parent Texture that this view was created from.
// Returns nil if the view has been released.
func (v *TextureView) Texture() *Texture {
	if v == nil || v.released || !v.surfaceTextureValid() {
		return nil
	}
	return v.texture
}

// Release marks the texture view for destruction. The underlying HAL TextureView
// (and its descriptor heap slots) is not freed immediately — it is deferred via
// DestroyQueue until the GPU completes any submission that may reference it.
// This prevents descriptor use-after-free on DX12 with maxFramesInFlight=2
// (BUG-DX12-007).
func (v *TextureView) Release() {
	if v.released {
		return
	}
	if v.surfaceToken != nil && (!v.surfaceToken.isValid() || v.device == nil || v.device.released.Load()) {
		v.released = true
		return
	}
	v.released = true

	if v.device == nil {
		return
	}

	halDevice := v.device.halDevice()
	if halDevice == nil {
		return
	}

	dq := v.device.destroyQueue()
	if dq == nil {
		halDevice.DestroyTextureView(v.hal)
		return
	}

	subIdx := v.device.lastSubmissionIndex()
	halTV := v.hal
	dq.Defer(subIdx, "TextureView", func() {
		halDevice.DestroyTextureView(halTV)
	})
}
