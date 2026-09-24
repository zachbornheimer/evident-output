package review_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// API-062 shape coverage: the ways real embedders spell "an Isolated
// Output" and "the model runs inside it". Each fixture names the line the
// finding must anchor to — the package-level declaration itself.

// isolatedHeader is the shared package clause and imports.
const isolatedHeader = `package p

import (
	"context"
	"io"
	"net/http"

	evo "github.com/zachbornheimer/evident-output"
)

`

// The literal spec §53 sample: the handler calls a model function declared
// elsewhere in the package, and the package-level evo.Task lives there.
const spec53SampleSrc = isolatedHeader + `type Agent struct{ Name string }

func defaultAgent() Agent { return Agent{Name: "agent"} }

func launchAgent(ctx context.Context, agent Agent) error {
	evo.Task("load " + agent.Name).Define(func(context.Context) error { return nil })
	return nil
}

func handle(w http.ResponseWriter, r *http.Request) {
	out := evo.Init(evo.Config{
		Isolated: true,
		Format:   evo.FormatExternal,
		Stdout:   io.Discard,
		Stderr:   io.Discard,
	})

	result := out.Run(r.Context(), func(ctx context.Context) error {
		return launchAgent(ctx, defaultAgent())
	})

	if err := evo.WriteJSON(w, result); err != nil {
		_ = err
	}
}
`

func TestAPI062_Spec53Sample_FiresInsideTheModelFunction(t *testing.T) {
	f := findingByID(t, review.GoSource("handler.go", spec53SampleSrc), "API-062")
	if f.Line != 16 {
		t.Fatalf("API-062 line = %d, want 16 (evo.Task inside launchAgent)", f.Line)
	}
}

// Every recognized spelling of the Isolated Output, and of the callback
// Run executes, fires at the package-level evo.Task.
func TestAPI062_RecognizedShapes_Fire(t *testing.T) {
	cases := map[string]struct {
		src  string
		line int
	}{
		"var declaration": {line: 14, src: `func handle(ctx context.Context) {
	var out = evo.Init(evo.Config{Isolated: true})
	out.Run(ctx, func(ctx context.Context) error {
		evo.Task("load agent").Define(func(context.Context) error { return nil })
		return nil
	})
}
`},
		"config held in a variable": {line: 15, src: `func handle(ctx context.Context) {
	cfg := evo.Config{Isolated: true, Stdout: io.Discard}
	out := evo.Init(cfg)
	out.Run(ctx, func(ctx context.Context) error {
		evo.Task("load agent").Define(func(context.Context) error { return nil })
		return nil
	})
}
`},
		"Isolated from a variable": {line: 14, src: `func handle(ctx context.Context, isolated bool) {
	out := evo.Init(evo.Config{Isolated: isolated})
	out.Run(ctx, func(ctx context.Context) error {
		evo.Task("load agent").Define(func(context.Context) error { return nil })
		return nil
	})
}
`},
		"Output parameter": {line: 13, src: `func serve(ctx context.Context, out *evo.Output) {
	out.Run(ctx, func(ctx context.Context) error {
		evo.Task("load agent").Define(func(context.Context) error { return nil })
		return nil
	})
}
`},
		"handler field": {line: 17, src: `type handler struct {
	out *evo.Output
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.out.Run(r.Context(), func(ctx context.Context) error {
		evo.Task("load agent").Define(func(context.Context) error { return nil })
		return nil
	})
}
`},
		"package-level Output": {line: 15, src: `var out = evo.Init(evo.Config{Isolated: true})

func handle(ctx context.Context) {
	out.Run(ctx, func(ctx context.Context) error {
		evo.Task("load agent").Define(func(context.Context) error { return nil })
		return nil
	})
}
`},
		"named run function": {line: 17, src: `func handle(ctx context.Context) {
	out := evo.Init(evo.Config{Isolated: true})
	out.Run(ctx, run)
}

func run(ctx context.Context) error {
	evo.Task("load agent").Define(func(context.Context) error { return nil })
	return nil
}
`},
		"callee of a callee": {line: 22, src: `func handle(ctx context.Context) {
	out := evo.Init(evo.Config{Isolated: true})
	out.Run(ctx, func(ctx context.Context) error { return launch(ctx) })
}

func launch(ctx context.Context) error {
	register()
	return nil
}

func register() {
	evo.Task("register agent").Define(func(context.Context) error { return nil })
}
`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := findingByID(t, review.GoSource("handler.go", isolatedHeader+tc.src), "API-062")
			if f.Line != tc.line {
				t.Fatalf("API-062 line = %d, want %d", f.Line, tc.line)
			}
		})
	}
}

// Shapes that must stay silent: an explicitly non-isolated Config, a
// model that already declares on the Output it is handed, and a recursive
// model (following callees must terminate).
func TestAPI062_UnrecognizedOrCorrectShapes_Silent(t *testing.T) {
	cases := map[string]string{
		"Isolated: false": `func handle(ctx context.Context) {
	out := evo.Init(evo.Config{Isolated: false})
	out.Run(ctx, func(ctx context.Context) error {
		evo.Task("load agent").Define(func(context.Context) error { return nil })
		return nil
	})
}
`,
		"model declares on its Output": `func handle(ctx context.Context) {
	out := evo.Init(evo.Config{Isolated: true})
	out.Run(ctx, func(ctx context.Context) error { return launch(ctx, out) })
}

func launch(ctx context.Context, out *evo.Output) error {
	out.Task("load agent").Define(func(context.Context) error { return nil })
	return nil
}
`,
		"recursive model": `func handle(ctx context.Context) {
	out := evo.Init(evo.Config{Isolated: true})
	out.Run(ctx, func(ctx context.Context) error { return walk(ctx, out, 3) })
}

func walk(ctx context.Context, out *evo.Output, depth int) error {
	if depth == 0 {
		return nil
	}
	out.Task("step").Define(func(context.Context) error { return nil })
	return walk(ctx, out, depth-1)
}
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			assertNoFinding(t, review.GoSource("handler.go", isolatedHeader+src), "API-062")
		})
	}
}

// Directory review follows a model function into another file of the same
// package — the layout examples/launch-agent-http uses.
func TestAPI062_Directory_FollowsModelIntoAnotherFile(t *testing.T) {
	dir := t.TempDir()
	handler := isolatedHeader + `func handle(w http.ResponseWriter, r *http.Request) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	result := out.Run(r.Context(), func(ctx context.Context) error { return launchAgent(ctx) })
	_ = evo.WriteJSON(w, result)
}
`
	agent := `package p

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func launchAgent(ctx context.Context) error {
	evo.Task("load agent").Define(func(context.Context) error { return nil })
	return nil
}
`
	writeFixture(t, filepath.Join(dir, "handler.go"), handler)
	writeFixture(t, filepath.Join(dir, "agent.go"), agent)

	res, err := review.GoDirectory(dir)
	if err != nil {
		t.Fatalf("GoDirectory: %v", err)
	}
	f := findingByID(t, res, "API-062")
	if filepath.Base(f.File) != "agent.go" || f.Line != 10 {
		t.Fatalf("API-062 at %s:%d, want agent.go:10", f.File, f.Line)
	}
}

func writeFixture(t *testing.T, path, src string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
