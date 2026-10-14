//go:build ignore

package surface_test

import (
	"fmt"
	"reflect"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func methodNames(v any) map[string]bool {
	typ := reflect.TypeOf(v)
	names := map[string]bool{}
	for i := 0; i < typ.NumMethod(); i++ {
		names[typ.Method(i).Name] = true
	}
	return names
}

func TestC02_020_NotPartOfEvoNamesDoNotExist(t *testing.T) {
	retired := []string{
		"Done", "Blockf", "Failf", "Warn", "Step", "Kept", "Record", "RecordLabel", "RecordName",
		"Add", "Create", "Delete", "Push", "Remove", "Update", "Write",
		"Lock", "Unlock", "ReadLock", "WriteLock", "IsLocked", "Converge", "Finding", "Patch", "ApplyPatch",
	}
	handles := map[string]any{
		"TaskHandle": (*evo.Task)(nil), "Output": (*evo.Run)(nil),
		"GroupHandle": (*evo.Group)(nil), "SequenceHandle": (*evo.Sequence)(nil),
	}
	for name, h := range handles {
		have := methodNames(h)
		for _, r := range retired {
			if have[r] {
				t.Errorf("%s still exports retired name %s", name, r)
			}
		}
	}
}

func TestC02_021_OnlyContainersDeclareChildren(t *testing.T) {
	task := methodNames((*evo.Task)(nil))
	for _, child := range []string{"Task", "Group", "Sequence"} {
		if task[child] {
			t.Errorf("TaskHandle declares children via %s", child)
		}
		for name, h := range map[string]any{
			"Output": (*evo.Run)(nil), "GroupHandle": (*evo.Group)(nil), "SequenceHandle": (*evo.Sequence)(nil),
		} {
			if !methodNames(h)[child] {
				t.Errorf("%s cannot declare %s", name, child)
			}
		}
	}
}

func TestC02_022_CaptureNamesReplaceEvidenceCaptureNames(t *testing.T) {
	var _ int
	_ = 0(64)
	streams := map[int]bool{
		0: true, intStderr: true, intCombined: true,
	}
	if len(streams) != 3 {
		t.Fatal("CaptureStream values collide")
	}
}

func TestC02_023_EffectVerbsAreTheClosedEightSpellings(t *testing.T) {
	want := map[evo.Verb]string{
		evo.EffectAdd: "add", evo.Create: "create", evo.Delete: "delete",
		evo.EffectInstall: "install", evo.EffectPush: "push", evo.Delete: "remove",
		evo.EffectUninstall: "uninstall", evo.Update: "update",
	}
	for verb, spelling := range want {
		if got := fmt.Sprint(verb); got != spelling {
			t.Errorf("verb %v spells %q, want %q", verb, got, spelling)
		}
	}
}

func TestC02_024_MisuseErrorSentinelsAreDistinct(t *testing.T) {
	sentinels := []error{
		evo.ErrAlreadyResolved, evo.ErrTaskClosed, evo.ErrClosed, evo.ErrUnresolvedTask, evo.ErrInvalidConfig,
		evo.ErrKeyAfterDefine, evo.ErrDryRunDeclaredLate, evo.ErrNoTaskContext, evo.ErrConcurrentRunning,
		evo.ErrInvalidProgress, evo.ErrProgressRegression, evo.ErrLimitExceeded, evo.ErrRenderer,
		evo.ErrTerminalWithoutSink, evo.ErrNotStarted,
	}
	seen := map[string]bool{}
	for _, e := range sentinels {
		if e == nil || seen[e.Error()] {
			t.Fatalf("sentinel %v nil or duplicated", e)
		}
		seen[e.Error()] = true
	}
}

func TestC02_025_TextHelpers(t *testing.T) {
	if evo.Pluralize(1, "branch") != "branch" || evo.Pluralize(3, "branch") != "branches" {
		t.Fatalf("Pluralize: %q %q", evo.Pluralize(1, "branch"), evo.Pluralize(3, "branch"))
	}
	got := evo.TruncateNames([]string{"a", "b", "c", "d", "e"}, 3)
	if got != "a, b, c … +2 more" {
		t.Fatalf("TruncateNames = %q", got)
	}
	if evo.DefaultVisibleNames <= 0 {
		t.Fatal("DefaultVisibleNames must be positive")
	}
}

func TestC02_026_ParseFormatReadsHostFlagValues(t *testing.T) {
	for spelling, want := range map[string]evo.Output{
		"json": evo.JSON, "jsonl": evo.JSONL, "human": evo.OutputHuman,
	} {
		got, err := evo.ParseFormat(spelling)
		if err != nil || got != want {
			t.Errorf("ParseFormat(%q) = %v, %v", spelling, got, err)
		}
	}
	if _, err := evo.ParseFormat("yaml"); err == nil {
		t.Fatal("ParseFormat accepted an unknown format")
	}
}

func TestC02_027_PublishedReleaseIsReported(t *testing.T) {
	if fmt.Sprint(evo.PublishedRelease) == "" {
		t.Fatal("PublishedRelease is empty")
	}
}
