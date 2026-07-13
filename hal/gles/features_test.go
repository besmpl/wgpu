// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build (windows || linux) && !(js && wasm)

package gles

import (
	"testing"

	"github.com/gogpu/gputypes"
)

func TestGLESDoesNotAdvertiseIndexedMultiDraw(t *testing.T) {
	// Include the timer-query extension so queryFeatures can run without a
	// live GL context; the MDI decision is extension/version independent and
	// must remain absent on GLES and desktop GL routes.
	const indexedMDI = gputypes.Features(gputypes.FeatureMultiDrawIndirect | gputypes.FeatureIndirectFirstInstance)
	tests := []struct {
		name  string
		major int
		minor int
		isES  bool
		want  gputypes.Features
	}{
		{
			name:  "desktop GL",
			major: 4,
			minor: 6,
			want:  gputypes.Features(gputypes.FeatureIndirectFirstInstance),
		},
		{name: "GLES", major: 3, minor: 2, isES: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			features := queryFeatures(
				map[string]bool{"GL_ARB_timer_query": true, "GL_ARB_shader_draw_parameters": true},
				tt.major, tt.minor, tt.isES, nil,
			)
			if got := features & indexedMDI; got != tt.want {
				t.Fatalf("indexed MDI bits = %#x, want %#x", got, tt.want)
			}
		})
	}
}
