//go:build !rust && !(js && wasm)

package wgpu

import (
	"fmt"

	"github.com/gogpu/gputypes"
)

func validateAndroidInstanceFlags(flags gputypes.InstanceFlags) error {
	if flags&gputypes.InstanceFlagsDebug != 0 {
		return fmt.Errorf("wgpu: debug callbacks are unsupported on Android")
	}
	return nil
}
