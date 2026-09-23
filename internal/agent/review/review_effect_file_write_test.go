package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// API-053 (ZYS-932): a filesystem mutator call hidden inside an
// evo.Effect callback bypasses evo.File/evo.Patch.

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

func TestAPI053_RawWriteFileInEffectLiteral_Fires(t *testing.T) {
	res := review.GoSource("configure.go", rawWriteFileInEffectLiteralSrc)
	f := findingByID(t, res, "API-053")
	if f.Severity != "error" {
		t.Fatalf("API-053 severity = %q, want error", f.Severity)
	}
	if !strings.Contains(f.Suggestion, "evo.File") {
		t.Fatalf("API-053 suggestion does not name evo.File: %q", f.Suggestion)
	}
	if f.RequiredVersion != "1.1.0" {
		t.Fatalf("API-053 required_version = %q, want 1.1.0", f.RequiredVersion)
	}
}

// The mutation can be one layer further hidden: the Effect callback is a
// same-file named function, not a literal. EVO-DRYRUN-001's raw-mutation
// walk only recurses into text lexically inside the Define body, so it
// never resolves a bare identifier callback to its declaration elsewhere in
// the file — API-053 must.

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

func TestAPI053_RawWriteFileInEffectNamedFunc_Fires(t *testing.T) {
	res := review.GoSource("manifest.go", rawWriteFileInEffectNamedFuncSrc)
	findingByID(t, res, "API-053")
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

func TestAPI053_WriteModeOpenFileInEffect_Fires(t *testing.T) {
	res := review.GoSource("rotate.go", writeModeOpenFileInEffectSrc)
	findingByID(t, res, "API-053")
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

func TestAPI053_ReadOnlyOpenFileInEffect_StaysSilent(t *testing.T) {
	res := review.GoSource("push.go", readOnlyOpenFileInEffectSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-053" {
			t.Fatalf("false positive API-053 on a read-only os.OpenFile: %+v", f)
		}
	}
}

// When the callback also reads the same file's existing contents before
// writing it back, the Suggestion must name evo.Patch -> evo.Files, not a
// plain evo.File — the write derives from existing state.

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

func TestAPI053_WriteDerivedFromRead_SuggestsPatch(t *testing.T) {
	res := review.GoSource("transform.go", rawWriteDerivedFromReadInEffectSrc)
	f := findingByID(t, res, "API-053")
	if !strings.Contains(f.Suggestion, "evo.Patch") || !strings.Contains(f.Suggestion, "evo.Files") {
		t.Fatalf("API-053 suggestion does not name evo.Patch -> evo.Files for a derived write: %q", f.Suggestion)
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

func TestAPI053_RealOpaqueEffectCallback_StaysSilent(t *testing.T) {
	res := review.GoSource("prune.go", realOpaqueEffectCallbackSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-053" {
			t.Fatalf("false positive API-053 on a real opaque Effect callback: %+v", f)
		}
	}
}

// evo.File is the remediated shape and must never itself trigger API-053.

const evoFileGoodCodeSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(ctx context.Context, path string, contents []byte) error {
  return evo.File(ctx, evo.FileSpec{Path: path, Contents: contents, Mode: 0o644})
}
`

func TestAPI053_EvoFileGoodCode_StaysSilent(t *testing.T) {
	res := review.GoSource("configure_good.go", evoFileGoodCodeSrc)
	for _, f := range res.Findings {
		if f.RuleID == "API-053" {
			t.Fatalf("false positive API-053 on evo.File: %+v", f)
		}
	}
}

func TestAPI053_PreOneOneOnePin_StaysSilent(t *testing.T) {
	res := review.GoSourceAt("configure.go", rawWriteFileInEffectLiteralSrc, "1.0.0")
	for _, f := range res.Findings {
		if f.RuleID == "API-053" {
			t.Fatalf("API-053 fired for a pin older than 1.1.0 (evo.Effect did not exist yet): %+v", f)
		}
	}
}

func TestAPI053_RecheckAfterRemediation_FindingDisappears(t *testing.T) {
	res := review.GoSource("configure.go", rawWriteFileInEffectLiteralSrc)
	findingByID(t, res, "API-053")

	after := review.GoSource("configure_good.go", evoFileGoodCodeSrc)
	for _, f := range after.Findings {
		if f.RuleID == "API-053" {
			t.Fatalf("API-053 still fires after remediation to evo.File: %+v", f)
		}
	}
}
