package handler

import (
	"testing"

	"go-backend/internal/store/model"
)

func TestKernelSupportsPostQuantumReality(t *testing.T) {
	for _, tc := range []struct {
		kernelType, installed string
		want                  bool
	}{
		{"xray", "26.7.11", true},
		{"xray", "v26.7.11", true},
		{"xray", "26.7.12", true},
		{"xray", "26.10.29", true},
		{"xray", "27.1.0", true},
		{"xray", "Xray 26.7.11 (Xray, Penetrates Everything.)", true},
		{"xray", "Xray 26.10.29 (Xray, Penetrates Everything.)", true},
		{"xray", "  xRaY\tv26.7.11 (release)  ", true},
		{"xray", "25.5.16", false},
		{"xray", "v25.10.15", false},
		{"xray", "26.1", false},
		{"xray", "26.7", false},
		{"xray", "26.7.10", false},
		{"xray", "Xray 26.7.10 (old release)", false},
		// Unknown and malformed installed versions do not count.
		{"xray", "", false},
		{"xray", "latest", false},
		{"xray", "Xray", false},
		{"xray", "Xray latest", false},
		{"xray", "26.7.11.1", false},
		{"xray", "26.7.11-beta.1", false},
		{"xray", "other 26.7.11", false},
		{"singbox", "26.7.11", false},
		{"sing-box", "26.7.11", false},
	} {
		got := kernelSupportsPostQuantumReality(&model.NodeKernel{KernelType: tc.kernelType, InstalledVersion: tc.installed})
		if got != tc.want {
			t.Errorf("%s %q: got %v want %v", tc.kernelType, tc.installed, got, tc.want)
		}
	}
	if kernelSupportsPostQuantumReality(nil) {
		t.Error("missing kernel")
	}
}
