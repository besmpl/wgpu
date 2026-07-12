// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package dx12

import "testing"

func TestShouldEnumerateWarp(t *testing.T) {
	tests := []struct {
		name         string
		env          string
		adapterCount int
		want         bool
	}{
		{name: "disabled by default", adapterCount: 0, want: false},
		{name: "opted in without hardware", env: "1", adapterCount: 0, want: true},
		{name: "opted in with hardware", env: "1", adapterCount: 1, want: false},
		{name: "other value", env: "true", adapterCount: 0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(dx12WarpEnv, tt.env)
			if got := shouldEnumerateWarp(tt.adapterCount); got != tt.want {
				t.Fatalf("shouldEnumerateWarp(%d) = %v, want %v", tt.adapterCount, got, tt.want)
			}
		})
	}
}
