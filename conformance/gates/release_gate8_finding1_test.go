//go:build ignore

package gates_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestInit_WithOptionsInstallsDefault is release-gate round 8 finding 1:
// Init(Config{Options: ...}) must install its result as the package-level
// default exactly like every other Init call. Before this fix, the Options
// escape hatch never installed a default, so a later package-level
// evo.Task(...) silently lazy-built a SECOND, bare Output (no DryRun, no
// Title, no configured writer) — a dry-run program's [planned] rows and
// title band vanished from the buffer the caller actually configured, and
// "deleted 2 stale branches" printed to a second, unobserved instance
// instead of rendering as [planned].
//
// Config{Isolated: true} is the one and only opt-out; it is orthogonal to
// Options (see TestInit_WithOptionsAndIsolatedSkipsDefaultInstall).
func TestInit_WithOptionsInstallsDefault(t *testing.T) {
	var buf bytes.Buffer
	(*evo.Run)(nil)
	_ = evo.Execute(context.Background(), evo.Config{Name: "retire", DryRun: true, Output: evo.Plain, Facades: evo.Facades{Stdout: &buf}}, func(ctx context.Context, r *evo.Run) error { return nil })
	t.Cleanup(func() { evo.SetDefault(nil) })

	branches := evo.Task("branches")
	branches.Define(effectOf(evo.Delete, "stale branch", 2))

	if err := (*evo.Run)(nil).Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil", err)
	}

	got := buf.String()
	if !strings.Contains(got, "retire") {
		t.Fatalf("Title must render on the one installed instance, got:\n%s", got)
	}
	if !strings.Contains(got, "[dry-run]") {
		t.Fatalf("DryRun must render the dry-run banner on the installed instance, got:\n%s", got)
	}
	if !strings.Contains(got, "[planned]") {
		t.Fatalf("want [planned] section on the installed instance, got:\n%s", got)
	}
	collapsed := strings.Join(strings.Fields(got), " ")
	if !strings.Contains(collapsed, "delete 2 stale branches") {
		t.Fatalf("want the Delete mutation rendered on the one instance, got:\n%s", got)
	}
}

// TestInit_WithOptionsAndIsolatedSkipsDefaultInstall is the green
// counterpart: Config{Isolated: true} remains the one and only opt-out from
// default installation, orthogonal to Options.
func TestInit_WithOptionsAndIsolatedSkipsDefaultInstall(t *testing.T) {
	evo.SetDefault(nil)
	t.Cleanup(func() { evo.SetDefault(nil) })

	var buf bytes.Buffer
	out := (*evo.Run)(nil)
	_ = evo.Execute(context.Background(), evo.Config{Output: evo.Plain, Facades: evo.Facades{Stdout: &buf}}, func(ctx context.Context, r *evo.Run) error { return nil })

	if (*evo.Run)(nil) == out {
		t.Fatal("Isolated: true must never install the built Output as the package-level default")
	}
}
