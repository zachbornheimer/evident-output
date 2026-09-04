package evo_test

import (
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestEvidenceOption_NewAndDeprecatedNamesCompile is C9: the Capture->Evidence
// rename finishes in the option surface (EvidenceOption/EvidenceStream*/
// MaxEvidenceBytes are canonical); the v0.2.16-shipped CaptureOption/
// CaptureStream*/MaxCaptureBytes spellings stay as deprecated aliases.
func TestEvidenceOption_NewAndDeprecatedNamesCompile(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	defer func() { _ = out.Close() }()

	task := out.Task("build")

	// Canonical (new) spelling.
	_ = task.Evidence(evo.KeepLastLines(50), evo.MaxEvidenceBytes(4096))

	// Deprecated (v0.2.16) spelling still compiles and behaves identically.
	_ = task.Evidence(evo.MaxCaptureBytes(4096))
	if acceptsDeprecatedCaptureOption(evo.MaxCaptureBytes(4096)) == nil {
		t.Fatal("evo.MaxCaptureBytes should satisfy the deprecated evo.CaptureOption alias")
	}
	if evo.CaptureStreamStderr != evo.EvidenceStreamStderr {
		t.Fatalf("deprecated CaptureStreamStderr diverged from EvidenceStreamStderr")
	}
}

// acceptsDeprecatedCaptureOption's parameter is explicitly typed as the
// deprecated evo.CaptureOption alias, proving it is interchangeable with
// evo.EvidenceOption at the type level.
func acceptsDeprecatedCaptureOption(opt evo.CaptureOption) evo.CaptureOption {
	return opt
}
