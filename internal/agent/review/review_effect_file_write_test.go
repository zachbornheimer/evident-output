package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// API-054 (ZYS-932): a filesystem mutator call hidden inside an
// evo.Effect callback bypasses evo.File.

const rawWriteFileInEffectLiteralSrc = `package p
import (
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context, path string, contents []byte) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectUpdate, Object: "config file", Quantity: 1}, func(context.Context) error {
    return os.WriteFile(path, contents, 0o644)
  })
}
`

func TestAPI054_RawWriteFileInEffectLiteral_Fires(t *testing.T) {
	res := review.GoSource("configure.go", rawWriteFileInEffectLiteralSrc)
	f := findingByID(t, res, "API-054")
	if f.Severity != "error" {
		t.Fatalf("API-054 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "evo.File") {
		t.Fatalf("API-054 suggestion does not name evo.File: %q", f.Suggestion)
	}
	if f.RequiredVersion != "1.1.0" {
		t.Fatalf("API-054 required_version = %q, want 1.1.0", f.RequiredVersion)
	}
}

// The mutation can be one layer further hidden: the Effect callback is a
// same-file named function, not a literal. EVO-DRYRUN-001's raw-mutation
// walk only recurses into text lexically inside the Define body, so it
// never resolves a bare identifier callback to its declaration elsewhere in
// the file — API-054 must.

const rawWriteFileInEffectNamedFuncSrc = `package p
import (
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context, path string, contents []byte) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectCreate, Object: "manifest", Quantity: 1}, writeManifest)
}
func writeManifest(context.Context) error {
  return os.WriteFile("manifest.json", []byte("{}"), 0o644)
}
`

func TestAPI054_RawWriteFileInEffectNamedFunc_Fires(t *testing.T) {
	res := review.GoSource("manifest.go", rawWriteFileInEffectNamedFuncSrc)
	findingByID(t, res, "API-054")
}

// A write-mode os.OpenFile is just as much a raw filesystem mutator as
// os.WriteFile/os.Create, even though its name alone doesn't say so —
// detection must look at the flag argument, not just the callee name.

const writeModeOpenFileInEffectSrc = `package p
import (
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context, path string) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectUpdate, Object: "log file", Quantity: 1}, func(context.Context) error {
    f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
    if err != nil {
      return err
    }
    return f.Close()
  })
}
`

func TestAPI054_WriteModeOpenFileInEffect_Fires(t *testing.T) {
	res := review.GoSource("rotate.go", writeModeOpenFileInEffectSrc)
	findingByID(t, res, "API-054")
}

// A read-only os.OpenFile is not a mutator and must stay silent.

const readOnlyOpenFileInEffectSrc = `package p
import (
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context, path string) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectUpdate, Object: "remote ref", Quantity: 1}, func(context.Context) error {
    f, err := os.OpenFile(path, os.O_RDONLY, 0)
    if err != nil {
      return err
    }
    defer f.Close()
    return pushDerivedRef(f)
  })
}
`

func TestAPI054_ReadOnlyOpenFileInEffect_StaysSilent(t *testing.T) {
	res := review.GoSource("push.go", readOnlyOpenFileInEffectSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-054" {
			t.Fatalf("false positive API-054 on a read-only os.OpenFile: %+v", f)
		}
	}
}

// evo.Patch/evo.Files do not exist in this module's public API (ZYS-932):
// even when the callback also reads the same file's existing contents
// before writing it back, the Suggestion must still name evo.File, never a
// fictitious evo.Patch/evo.Files pair.

const rawWriteDerivedFromReadInEffectSrc = `package p
import (
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func run(ctx context.Context, path string) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectUpdate, Object: "config file", Quantity: 1}, func(context.Context) error {
    existing, err := os.ReadFile(path)
    if err != nil {
      return err
    }
    return os.WriteFile(path, transform(existing), 0o644)
  })
}
`

func TestAPI054_WriteDerivedFromRead_SuggestsEvoFile(t *testing.T) {
	res := review.GoSource("transform.go", rawWriteDerivedFromReadInEffectSrc)
	f := findingByID(t, res, "API-054")
	if !strings.Contains(f.Suggestion, "evo.File") {
		t.Fatalf("API-054 suggestion does not name evo.File for a derived write: %q", f.Suggestion)
	}
	if strings.Contains(f.Suggestion, "evo.Patch") || strings.Contains(f.Suggestion, "evo.Files(") {
		t.Fatalf("API-054 suggestion names the nonexistent evo.Patch/evo.Files API: %q", f.Suggestion)
	}
}

// An Effect callback doing real opaque work Evo cannot model declaratively
// (a remote API call, a git ref) is exactly what Effect is for and must
// stay silent.

const realOpaqueEffectCallbackSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(ctx context.Context, ref string) error {
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectDelete, Object: "remote ref", Quantity: 1}, func(context.Context) error {
    return deleteRemoteRef(ctx, ref)
  })
}
`

func TestAPI054_RealOpaqueEffectCallback_StaysSilent(t *testing.T) {
	res := review.GoSource("prune.go", realOpaqueEffectCallbackSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-054" {
			t.Fatalf("false positive API-054 on a real opaque Effect callback: %+v", f)
		}
	}
}

// evo.File is the remediated shape and must never itself trigger API-054.

const evoFileGoodCodeSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(ctx context.Context, path string, contents []byte) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: contents, Mode: 0o644})
}
`

func TestAPI054_EvoFileGoodCode_StaysSilent(t *testing.T) {
	res := review.GoSource("configure_good.go", evoFileGoodCodeSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-054" {
			t.Fatalf("false positive API-054 on evo.File: %+v", f)
		}
	}
}

func TestAPI054_PreOneOneOnePin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("configure.go", rawWriteFileInEffectLiteralSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-054" {
			t.Fatalf("API-054 fired for a pin older than 1.1.0 (evo.Effect did not exist yet): %+v", f)
		}
	}
}

func TestAPI054_RecheckAfterRemediation_FindingDisappears(t *testing.T) {
	res := review.GoSource("configure.go", rawWriteFileInEffectLiteralSrc)
	findingByID(t, res, "API-054")

	after := review.GoSource("configure_good.go", evoFileGoodCodeSrc)
	for _, f := range after.Findings {
		if f.RuleID == "API-054" {
			t.Fatalf("API-054 still fires after remediation to evo.File: %+v", f)
		}
	}
}
