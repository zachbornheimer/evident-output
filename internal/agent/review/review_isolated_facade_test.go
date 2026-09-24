package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// API-062 (ZYS-946, spec §53): package-level evo.Task/Group/Sequence/...
// inside an Isolated Output's Run callback declare on the package default,
// not on the Output being run — every concurrent HTTP request would then
// share one default's runtime state, and the request's own document would
// come back empty.

const isolatedRunPackageLevelTaskSrc = `package p

import (
	"context"
	"io"
	"net/http"

	evo "github.com/zachbornheimer/evident-output"
)

func handle(w http.ResponseWriter, r *http.Request) {
	out := evo.Init(evo.Config{Isolated: true, Format: evo.FormatExternal, Stdout: io.Discard, Stderr: io.Discard})
	result := out.Run(r.Context(), func(ctx context.Context) error {
		evo.Task("load agent").Define(func(context.Context) error { return nil })
		return nil
	})
	_ = evo.WriteJSON(w, result)
}
`

func TestAPI062_PackageLevelTaskInsideIsolatedRun_Fires(t *testing.T) {
	res := review.GoSource("handler.go", isolatedRunPackageLevelTaskSrc)
	f := findingByID(t, res, "API-062")
	if f.Line != 14 {
		t.Fatalf("API-062 line = %d, want 14 (the evo.Task call)", f.Line)
	}
	if !strings.Contains(f.Suggestion, "out.Task(") {
		t.Fatalf("API-062 suggestion does not spell the Output-bound call: %q", f.Suggestion)
	}
}

// The Output-bound spelling is the fix and stays silent.
const isolatedRunOutputBoundTaskSrc = `package p

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func handle(ctx context.Context) evo.Result {
	out := evo.Init(evo.Config{Isolated: true})
	return out.Run(ctx, func(ctx context.Context) error {
		out.Sequence("launch agent").Task("load agent").Define(func(ctx context.Context) error {
			return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectInstall, Object: "launch agent", Quantity: 1}, load)
		})
		return nil
	})
}

func load(context.Context) error { return nil }
`

func TestAPI062_OutputBoundDeclarationsInsideIsolatedRun_Silent(t *testing.T) {
	res := review.GoSource("handler.go", isolatedRunOutputBoundTaskSrc)
	assertNoFinding(t, res, "API-062")
}

// The ordinary CLI shape — package default Init, then package-level Task —
// is the teaching ladder's first rung and must stay silent.
const defaultInitPackageLevelTaskSrc = `package p

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	evo.Init(evo.Config{Title: "tool"})
	os.Exit(evo.Main(func(ctx context.Context) error {
		evo.Task("load agent").Define(func(context.Context) error { return nil })
		return nil
	}))
}
`

func TestAPI062_PackageLevelTaskUnderDefaultInit_Silent(t *testing.T) {
	res := review.GoSource("main.go", defaultInitPackageLevelTaskSrc)
	assertNoFinding(t, res, "API-062")
}
