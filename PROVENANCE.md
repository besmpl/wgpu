# Hearth proof fork provenance

This owned `github.com/besmpl/wgpu` fork temporarily carries two independent
surfaces used by Hearth's MDI migration: the maintainer-confirmed counted
indirect contract required for the upstream-only renderer, and the older
prepared/ICB/material-page path retained only as a comparison oracle until the
fork-removal phase. The source provenance remains the upstream
`github.com/gogpu/wgpu` snapshot recorded below.

This source is published as `v0.31.0-hearth.7` from the owned
`codex/material-page-v0310-hearth7-count` branch. It extends
`v0.31.0-hearth.6` (`4a509369350b199e9153bc7ef3818fe4ed331051`) with the
confirmed count contract at
`ba3c2446f5902e429d25b096741c394cede9f203`. The prerelease identifies these
fork bytes without moving or reusing an immutable upstream tag.

- Upstream repository: https://github.com/gogpu/wgpu
- Upstream base tag: `v0.30.4`
- Upstream base commit: `fe4e452cd9609c5bf0e18b533563ec751e926e72`
- Upstream module sum: `h1:gPy41KKySNS5kcIpolr4jC3FBGZIzSdFlbOLSV+eU6s=`
- Confirmed upstream count implementation:
  `ec7325206ca1af5ad96d8651b91bb1bb3165d859`
- Required DX12 ABI prerequisite:
  `ecb2480b822cefd70e2177c72e8cc98c39553efb`
- Prepared-path pull request: https://github.com/gogpu/wgpu/pull/253
- Maintainer contract: https://github.com/gogpu/wgpu/pull/253#issuecomment-4970690180
- Prerequisite pull requests: #257, #258, #260, #261, and draft #262
- Read-only upstream state checked: 2026-07-14; upstream `main` =
  `202e12574feb33f4ef052f1f6ffa693477eb75d7`; PR #253 remains open at
  `46096ae`.

The accepted contract preserves the existing two-argument public
`DrawIndirect` and `DrawIndexedIndirect` methods and adds
`MultiDrawIndirect` and `MultiDrawIndexedIndirect`. The existing HAL methods
gain `drawCount`. Count zero is a valid no-op; consecutive records use fixed
16-byte and 20-byte strides. DX12 uses native `ExecuteIndirect`; Vulkan uses a
native operation when supported and an exact advancing loop otherwise; Metal,
GLES, browser, and Rust use exact loops. `FeatureMultiDrawIndirect` is a
performance hint, not a semantic gate, and `FeatureMultiDrawIndirectCount`
remains reserved for future GPU-driven count buffers.

This fork remains based on upstream `v0.30.4`, so the confirmed change was
ported without importing later upstream history or the later goffi ABI. The
existing prepared indexed seam, Metal ICB translator, material page, texture
array correction, autorelease-thread fix, Vulkan layout correction, DX12
transition correction, and DX12 indirect descriptor ABI fix remain intact.
On the Apple M1 host, the physical counted/prepared/individual parity proof
passes:

```text
HEARTH_MATERIAL_PAGE_PROBE=1 CGO_ENABLED=0 go test \
  -tags hearth_material_page_probe -count=1 \
  -run '^TestMaterialPageMetalICBProbe$' -v .
```

The same source passes native cgo-free tests, Rust-tag tests, browser/Wasm
tests, Windows-amd64 compilation, and Linux-amd64 GLES compilation. Core
count-one and count-N forwarding both remain about 23 ns/op with zero
allocations on the recorded Apple M1 host. Physical Windows DX12, Linux GLES,
and Vulkan execution remain separate host evidence.

The Apple M1 benchmark gate ran five samples across 192 scenarios (960 rows).
The tagged `hearth_gpu_timing_probe` reports completed
`MTLCommandBuffer.GPUStartTime`/`GPUEndTime` intervals across the full submitted
batch; observed GPU time ranged from 64,958 to 10,159,333 ns per operation.
The measurements confirm that prepared execution wins for large homogeneous
sets but must not be forced across fragmented material/arena sets.

The counted source commit's tracked tree, excluding this provenance note, is
identified by this reproducible digest:

```text
git ls-tree -r --full-tree ba3c244 |
  awk '$4 != "PROVENANCE.md" { print $3, $4 }' | shasum -a 256
f2fce85d7df24c09b4fb3b2852d5845a2b1da47169c68771d7fd0fcaa6430d2b
```

The source is intentionally not byte-identical to any single upstream pull
request: it combines the confirmed counted contract with Hearth's temporary
comparison oracle on the older base. Unsupported routes must not advertise the
prepared capability, while counted semantics remain available regardless of
the performance hint.

Refresh procedure: start from the selected upstream tag, reapply only the
minimal confirmed API/HAL delta still missing, recompute the tracked-tree
digest, and run the native, Rust, browser, Windows, Linux/GLES, and physical
host checks above. Removal procedure: once an upstream release carries the
confirmed contract, switch Hearth to that release and delete the prepared,
ICB, material-page, receipt, token, and fork-only capability surface in the
planned removal phase.
