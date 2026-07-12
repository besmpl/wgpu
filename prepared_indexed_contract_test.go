//go:build !rust && !(js && wasm)

package wgpu

import (
	"errors"
	"testing"
)

func TestPreparedIndexedValidationRejectsMalformedBoundaryInputs(t *testing.T) {
	tests := []struct {
		name string
		desc PreparedIndexedCommandsDescriptor
		want error
	}{
		{name: "nil arguments", desc: PreparedIndexedCommandsDescriptor{Count: 1}, want: ErrPreparedIndexedInvalid},
		{name: "zero count", desc: PreparedIndexedCommandsDescriptor{Count: 0}, want: ErrPreparedIndexedInvalid},
		{name: "zero-value buffer handles", desc: PreparedIndexedCommandsDescriptor{
			Arguments: &Buffer{}, IndexBuffer: &Buffer{}, Count: 1,
		}, want: ErrPreparedIndexedInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validatePreparedIndexedDescriptor(tt.desc, 0); !errors.Is(err, tt.want) {
				t.Fatalf("validation error = %v, want %v", err, tt.want)
			}
		})
	}
}
