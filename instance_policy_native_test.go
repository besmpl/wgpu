//go:build !rust && !(js && wasm)

package wgpu

import (
	"testing"

	"github.com/gogpu/gputypes"
)

func TestValidateAndroidInstanceFlagsRejectsDebug(t *testing.T) {
	if err := validateAndroidInstanceFlags(gputypes.InstanceFlagsDebug); err == nil {
		t.Fatal("Android debug flag was accepted")
	}
	if err := validateAndroidInstanceFlags(gputypes.InstanceFlagsValidation); err != nil {
		t.Fatalf("Android validation-only flags rejected: %v", err)
	}
}
