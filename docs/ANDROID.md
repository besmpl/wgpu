# Android Vulkan preview

Android/arm64 support is currently an implementation preview, not a released
support claim. The native backend is Vulkan-only, requires Android API 29 or
newer and a Vulkan 1.2 driver, and deliberately rejects debug callbacks. GLES,
software fallback, 32-bit Android, and API 28 or earlier are out of scope.

The preview depends on the unreleased Android work in
[go-webgpu/goffi#62](https://github.com/go-webgpu/goffi/pull/62), pinned in CI
to commit `3d665de6d43af35dd6dae005ef09231c15b0d456`. The module file does not use
a fork, `replace`, or committed workspace overlay. CI checks out that canonical
PR head into an ephemeral `go.work` only for integration proof.

## Host contract

`Instance.CreateSurface` keeps its existing two-`uintptr` API on Android:

- `displayHandle` is a non-zero, monotonically increasing window generation.
- `windowHandle` is the raw `ANativeWindow*` value.

The host owns its Activity lifecycle, the generation counter, and its own
application reference to the window. On successful `vkCreateAndroidSurfaceKHR`,
Vulkan acquires the reference owned by the `VkSurfaceKHR`; Vulkan releases that
reference from `vkDestroySurfaceKHR`. WGPU therefore never calls
`ANativeWindow_acquire` or `ANativeWindow_release`.

Create a new surface after Android replaces the native window, using a larger
generation, and release the old `Surface`. Requests using an older generation
fail with `hal.ErrSurfaceLost`. A generation identifies a window lifetime; it
is not a display pointer.

## Rust wgpu v29 parity

The semantic oracle is gfx-rs/wgpu v29.0.3 commit
`4cbe6232b2d7c289b6e1a38416a6ae1461a22e81`. The implementation follows its
Android policy without copying Rust code or changing the Go public API.

| Behavior | Rust v29 oracle | Go implementation and proof |
|----------|-----------------|-----------------------------|
| Android WSI extension and raw window | `wgpu-hal/src/vulkan/instance.rs` | `api_android.go`, `android_surface_policy_test.go` |
| API 29 acquire uses an infinite timeout; API 30+ preserves the request | `swapchain/native.rs::acquire` | `swapchainPlatformPolicy.acquireTimeout`, `swapchain_platform_test.go` |
| Fence waits use `waitAll=true` | `swapchain/native.rs::acquire` | checked acquire wait in `swapchain.go` |
| Android uses identity pre-transform and ignores orientation-only suboptimal status | `swapchain/native.rs::{create_swapchain,acquire,present}` | `preTransform`, `reportSuboptimal`, platform-policy tests |
| Fixed `currentExtent` is authoritative; variable extents are clamped | `swapchain/native.rs::surface_capabilities` | `selectSwapchainExtent`, swapchain policy tests |
| `Rgba16Float` uses extended-linear sRGB; other formats use nonlinear sRGB | `swapchain/native.rs::create_swapchain`, `conv.rs::map_vk_surface_formats` | exact format/color-space pair selection tests |
| Swapchain resources drain before native device/instance teardown | `NativeSwapchain::release_resources` | public and Vulkan lifecycle registries, `surface_lifecycle_test.go`, `instance_lifecycle_native_test.go` |

The Go backend is intentionally stricter where errors are observable: required
instance/device extensions and commands must be present, enumeration retries
are bounded, Vulkan status values map to typed HAL errors, and teardown or
layout-transition failures do not fabricate success.

## Dependency stack for this preview

This branch incorporates the reviewed patch stacks at the exact heads of four
independently reviewable WGPU prerequisites. Their commits remain visible in
the history. Commits that overlap the Android work are conflict-resolved on the
stacked branch, while non-overlapping commits retain matching stable patch IDs.
The branch must be rebased to Android-only commits after those changes land:

| Prerequisite | Upstream PR | Exact head |
|--------------|-------------|------------|
| Drain GPU work before device teardown | [gogpu/wgpu#264](https://github.com/gogpu/wgpu/pull/264) | `0ed17064f8c977f35d9b49b5cde0d0c69e867ecf` |
| Fail closed during swapchain negotiation and synchronization | [gogpu/wgpu#265](https://github.com/gogpu/wgpu/pull/265) | `a8ff52e340f0a06c8e1b6599a03856d7fc74d1a2` |
| Never manufacture a production mock adapter | [gogpu/wgpu#266](https://github.com/gogpu/wgpu/pull/266) | `e97e4901ee76d9e5f587569c073b38114221c4e6` |
| Qualify a request-local same-family graphics/present queue | [gogpu/wgpu#267](https://github.com/gogpu/wgpu/pull/267) | `a3e839f94a12edce98e2496d96e5bd8d3cdd2fc3` |

## Reproducing deterministic proof

Use Android NDK r29 and a clean checkout of the exact goffi candidate:

```bash
GOFFI_DIR=/path/to/goffi \
GOFFI_EXPECTED_HEAD=3d665de6d43af35dd6dae005ef09231c15b0d456 \
ANDROID_NDK_HOME=/path/to/android-ndk-r29 \
GOTOOLCHAIN=go1.26.5 \
./scripts/check-android-arm64-preview.sh
```

The script runs the full Android/arm64 package test/build selection with
`CGO_ENABLED=0` and `CGO_ENABLED=1`, builds the headless triangle, and audits
its ELF dependencies. It requires Bionic `libc.so`/`libdl.so`, confirms the
runtime Vulkan loader name, and rejects glibc sonames/symbols, standalone
`libpthread`, and desktop WSI libraries. CI repeats it on Go 1.25.12 and
Go 1.26.5.

These checks prove source selection, cross-compilation, ABI guards, and linked
ELF shape. They do **not** prove process startup, real adapter enumeration,
surface creation, rendering, presentation, orientation behavior, or lifecycle
recovery on a physical Android device. Both cgo modes are cross-build evidence;
neither is a physical WSI support label.

Before this preview can become merge-ready, goffi#62 must merge and ship an
immutable canonical release, all prerequisite WGPU PRs must merge, this branch
must be rebased to Android-only commits using that release, and API 29 plus API
30-or-newer arm64 devices must pass zero-cgo startup, Vulkan rendering, cgo
comparison, first-frame/orientation checks, and repeated window replacement.
