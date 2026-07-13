# Hearth material-page ABI

This change selects Candidate B from Hearth's Metal ICB plan: explicit MSL
source carried by `ShaderModuleDescriptor.MSL` and marked with
`ShaderModuleDescriptor.MaterialPage`. The pinned naga v0.17.15 lowering emits
independent binding-array wrappers and cannot safely group a texture array and
sampler, so no generic Naga binding model was changed.

The marked pipeline carries one immutable `MaterialPageDescriptor`:

| Field | Value/meaning |
| --- | --- |
| `ABIVersion` | fixed ABI version (`1`) |
| `BindGroupIndex` | page placeholder group |
| `TextureBinding`, `SamplerBinding` | source shader bindings |
| `TextureArgumentIndex`, `SamplerArgumentIndex` | argument-buffer IDs |
| `FragmentBufferIndex` | reserved Metal fragment slot (`7`) |

The only operations are capability query, pipeline-bound page creation,
active-pass binding, and normal idempotent release. The optional HAL provider
owns Metal argument-buffer allocation, child-resource retention, residency
declaration, and destruction; browser, Rust, and non-Metal backends fail closed.
The page fingerprint includes ABI fields, layout/resource shape, and entry
points so the flagged and unflagged variants can share a page only when their
complete contract matches.

The losing bounded Naga prototype was deleted. Publication requires this WGPU
change to be released as the next owned module version (`.4` after
`v0.31.0-hearth.3`) and Hearth to consume that published checksum; no local
replacement is a finished state.
