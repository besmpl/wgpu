package wgpu

import "testing"

func TestMaterialPageDescriptorValidationAndFingerprint(t *testing.T) {
	base := MaterialPageDescriptor{
		ABIVersion: 1, BindGroupIndex: 1, TextureBinding: 4, SamplerBinding: 5,
		TextureArgumentIndex: 0, SamplerArgumentIndex: 1, FragmentBufferIndex: 7,
	}
	if !base.valid() {
		t.Fatal("complete material-page descriptor should be valid")
	}
	if base.fingerprint() == 0 {
		t.Fatal("complete material-page descriptor must have a compatibility fingerprint")
	}
	changed := base
	changed.SamplerBinding++
	if changed.fingerprint() == base.fingerprint() {
		t.Fatal("changing a binding must invalidate the compatibility fingerprint")
	}
	invalid := base
	invalid.SamplerArgumentIndex = invalid.TextureArgumentIndex
	if invalid.valid() {
		t.Fatal("texture and sampler argument IDs must be distinct")
	}
}

func TestMaterialPageCapabilitiesDefaultToUnsupported(t *testing.T) {
	var caps MaterialPageCapabilities
	if caps.Supported || caps.ABIVersion != 0 || caps.FragmentBufferIndex != 0 {
		t.Fatalf("zero capability must fail closed: %+v", caps)
	}
}

func TestMaterialPageDescriptorReachesHALMetadata(t *testing.T) {
	d := &ShaderModuleDescriptor{
		MSL:          "fragment float4 page() { return float4(1); }",
		MaterialPage: &MaterialPageShaderDescriptor{ABIVersion: 1, TextureArgumentIndex: 0, SamplerArgumentIndex: 1, FragmentBufferIndex: 7},
	}
	if d.MSL == "" || d.MaterialPage == nil || d.MaterialPage.FragmentBufferIndex != 7 {
		t.Fatalf("material-page metadata was not preserved: %+v", d)
	}
}

func TestMaterialPageShaderSourceFingerprintDistinguishesShaders(t *testing.T) {
	first := shaderSourceFingerprint(&ShaderModuleDescriptor{MSL: "fragment float4 fs() { return float4(1); }"})
	second := shaderSourceFingerprint(&ShaderModuleDescriptor{MSL: "fragment float4 fs() { return float4(0); }"})
	if first == 0 || second == 0 || first == second {
		t.Fatalf("shader source fingerprints must distinguish explicit sources: %d %d", first, second)
	}
	if same := shaderSourceFingerprint(&ShaderModuleDescriptor{MSL: "fragment float4 fs() { return float4(1); }"}); same != first {
		t.Fatalf("identical shader source fingerprint changed: %d != %d", same, first)
	}
}
