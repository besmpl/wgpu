# Hearth proof fork provenance

This directory is the owned source fork for the fixed-count indexed multi-draw
proof described by Hearth's MDI integration plan. Its module
path is `github.com/besmpl/wgpu`; the source provenance remains the upstream
`github.com/gogpu/wgpu` snapshot recorded below. Hearth imports the owned path
so a future publication cannot accidentally resolve the upstream module before
the prepared-indexed API is available.

This source is published as `v0.31.0-hearth.3` from the owned
`codex/hearth-v0.31.0-hearth.3` branch. Publication was explicitly authorized
on 2026-07-13. It retains the `v0.31.0-hearth.2` production fix that pins each Metal
autorelease-pool lifetime to one OS thread; physical depth-one lifecycle proof
exposed that Go goroutine migration could otherwise drain the Objective-C pool
on a different thread. It also fixes the Metal test's ownership of an
autoreleased render-pass descriptor so the published full suite is stable. The fork repository already contains upstream tags
through `v0.30.19`; in particular, `v0.30.4` points to the unmodified base
commit below and cannot identify this fork's different bytes. The prerelease
therefore names the breaking prepared-indexed API without moving or reusing an
immutable upstream tag.

- Upstream repository: https://github.com/gogpu/wgpu
- Upstream base tag: `v0.30.4`
- Upstream base commit: `fe4e452cd9609c5bf0e18b533563ec751e926e72`
- Upstream module sum: `h1:gPy41KKySNS5kcIpolr4jC3FBGZIzSdFlbOLSV+eU6s=`
- Hearth/upstream patch commit: `3bed88f9ba9185014d0eb621d2fba68fb9efaeab` (`fix(dx12): match indirect descriptor ABI size`), whose branch history starts with `bbe12f767707cc872a2e0e9890db9c4f04dcc616` (`add prepared indexed command path`)
- Upstream pull request: https://github.com/gogpu/wgpu/pull/253 (open for review)
- Upstream fork branch: `besmpl:codex/fixed-count-indexed-mdi`
- Read-only upstream state checked: 2026-07-13; PR `OPEN`, `isDraft=false`, no
  merge commit; `refs/pull/253/head` = `3bed88f9ba9185014d0eb621d2fba68fb9efaeab`
  and upstream `main` = `d5834be374cfef400606c5ca9160e4436aee2d1c`.

The pull request is based on upstream `main` commit `d5834be374cfef400606c5ca9160e4436aee2d1c`
(v0.30.19-era), while this Hearth proof copy remains the v0.30.4 source above.
This fork now also carries the encoder-time prepared indexed seam: public/core
validation, optional HAL preparation/execute interfaces, one-shot ownership,
direct Vulkan/DX12 forwarding, explicit unsupported browser/Rust/GLES/software
surfaces, and a cgo-free Metal ICB translator path guarded by a finite Apple
family/selector capability. Hearth's bounded texture-page proof also fixes the
v0.30.4 Metal descriptor translation so 1D/2D array layers map to
`arrayLength` while physical depth remains one; logical copy validation still
uses `DepthOrArrayLayers`. On the Apple M1 host, the untagged native visible
proof now passes:

```text
cd adapter/webgpu
GOWORK=off CGO_ENABLED=0 go test -tags hearth_prepared_indexed_probe -count=1 \
  -run TestRuntimeGPUMultiDrawIndexedIndirectNative -v ./render3d
```

The opt-in `hearth_prepared_indexed_probe` path additionally verifies real ICB
creation at the finite 52,428-command cap, translator creation/teardown, shared
encoder-owned arenas, and the visible proof's exact one-reset, one-dispatch,
one-execution-range command sequence. A public-API parity proof derived from
the same upstream implementation passed on Vulkan/lavapipe and DX12/WARP in
https://github.com/besmpl/wgpu/actions/runs/29211447974. That proof compared
two prepared records byte-for-byte with two individual indexed-indirect draws;
software drivers establish native backend correctness, not physical-GPU
performance. Hearth release activation now follows Route B: publish this
reviewed source at the owned module path and exact tag, then prove a clean
external consumer. A later merged upstream tag may replace that dependency in
a separate reviewed migration.

Vulkan capability normalization uses the physical device's
`MaxDrawIndirectCount`, DX12 treats baseline `ExecuteIndirect` as independent
of the Vulkan-style multi-draw feature gate, and Metal device teardown releases
its cached translator objects. Native proof also found and fixed Vulkan's
invalid nil-layout forwarding, the DX12 render-target readback transition, and
the DX12 indirect descriptor ABI size.

The Apple M1 benchmark gate ran five samples across 192 scenarios (960 rows).
The tagged `hearth_gpu_timing_probe` reports completed
`MTLCommandBuffer.GPUStartTime`/`GPUEndTime` intervals across the full submitted
batch; observed GPU time ranged from 64,958 to 10,159,333 ns per operation.
The measurements confirm that prepared execution wins for large homogeneous
sets but must not be forced across fragmented material/arena sets.

The published source starts from the exact tagged module revision above. Its
exact bytes (excluding this provenance note) are identified by this
reproducible digest:

```text
find . -type f ! -name PROVENANCE.md -print | LC_ALL=C sort |
  while IFS= read -r f; do shasum -a 256 "$f"; done | shasum -a 256
6365773e9a68b4e677d28bd9c8f8594779f17dea116d04572ae59d2a4404028e
```

The source carries the public/core/HAL prepared indexed operation, Vulkan
and DX12 native command forwarding, Metal's GPU-generated ICB preparation
path, and explicit unsupported methods for browser, Rust, GLES, software, and
noop routes. The source is intentionally not claimed to be byte-identical
to PR #253: the PR is based on `main` (`d5834be...`), while this proof copy is
based on `v0.30.4` and contains the additional prepared-route work used by
Hearth. The digest above, together with the base commit and this note, is the
exact proof input until an upstream release or an explicitly published fork
replaces it.
Unsupported routes must not advertise the prepared capability; Hearth must
retain its individual-indirect fallback.

Refresh procedure: replace this directory with a clean checkout of the chosen
upstream tag/commit, re-apply only the minimal API/HAL delta, recompute the
digest above, update this note, and run
`GOWORK=off CGO_ENABLED=0 go test ./...` here and in `adapter/webgpu`.
Removal procedure: once the owned module is published with the API, remove this
directory and the development `replace` directives atomically, set the direct
`github.com/besmpl/wgpu` requirement to that published version, regenerate
module sums normally, and run
`scripts/check-release-external-consumer <hearth-version> <wgpu-version>`.
