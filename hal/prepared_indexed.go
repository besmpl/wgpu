//go:build !(js && wasm)

package hal

import "github.com/gogpu/gputypes"

// IndexedBatchDescriptor describes one homogeneous range of tightly packed
// DrawIndexedIndirect records. It is deliberately backend-neutral; the HAL
// owns any prepared command object returned for the range.
type IndexedBatchDescriptor struct {
	Arguments       Buffer
	ArgumentsOffset uint64
	Count           uint32
	IndexBuffer     Buffer
	IndexOffset     uint64
	IndexFormat     gputypes.IndexFormat
}

// PreparedIndexedCommands is an opaque, encoder-owned backend token. The
// public wgpu layer never inspects or releases its value independently.
type PreparedIndexedCommands any

// PreparedIndexedCommandEncoder is optional. Backends that cannot prepare a
// native indexed multi-draw operation simply do not implement it; core then
// returns a typed unsupported error before recording any native commands.
type PreparedIndexedCommandEncoder interface {
	PrepareIndexed(IndexedBatchDescriptor) (PreparedIndexedCommands, error)
}

// PreparedIndexedRenderPass is optional and must be implemented together with
// PreparedIndexedCommandEncoder. Execution consumes one token exactly once.
type PreparedIndexedRenderPass interface {
	ExecutePreparedIndexed(PreparedIndexedCommands) error
}

// PreparedIndexedCapabilities reports a backend's complete prepared route.
// MaxCommands is zero when unsupported and positive finite when supported.
type PreparedIndexedCapabilities struct {
	PreparedIndexed bool
	MaxCommands     uint32
}

// PreparedIndexedCapabilityProvider is optional. It lets a HAL device expose
// a normalized route fact without making the base Device interface wider.
type PreparedIndexedCapabilityProvider interface {
	PreparedIndexedCapabilities() PreparedIndexedCapabilities
}

// PreparedIndexedFeatureGate lets a backend state whether its route is
// covered by the standard MultiDrawIndirect device feature. Private emulation
// routes (for example Metal's ICB translator) may opt out while direct native
// command routes require the feature to be explicitly enabled.
type PreparedIndexedFeatureGate interface {
	PreparedIndexedRequiresFeature() bool
}
