package tree_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/publish"
)

// pruneCrashAtChild runs a Replace that dies at the step pruneStepEnv
// names, optionally forced onto the move-aside path platforms without an
// atomic exchange take.
func pruneCrashAtChild(t *testing.T) int {
	dest := os.Getenv(pruneDestEnv)
	expected, archivePath, _ := splitPair(os.Getenv(pruneArgEnv))
	target, err := strconv.Atoi(os.Getenv(pruneStepEnv))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	publish.InjectFaults(publish.Faults{
		NoExchange: os.Getenv(pruneNoExchangeEnv) != "",
		At: func(step publish.Step, at string) {
			if at == dest && step == publish.Step(target) {
				os.Exit(0)
			}
		},
	})
	err = contractRun(t, evo.Config{}, func(ctx context.Context) error {
		return evo.Tree{Path: dest, Content: evo.Extract{File: evo.File{Path: archivePath}}}.Replace(ctx, expected)
	})
	fmt.Fprintf(os.Stderr, "Replace returned %v; the crash at step %d never ran\n", err, target)
	return 1
}

// crashReplace kills a child's Replace of f at step.
func (f pruneReplaceFixture) crashReplace(t *testing.T, step publish.Step, noExchange bool) {
	t.Helper()
	env := []string{
		pruneRoleEnv + "=" + pruneRoleCrashAt, pruneDestEnv + "=" + f.dest,
		pruneArgEnv + "=" + f.expected + "\n" + f.nextArchive(),
		pruneStepEnv + "=" + strconv.Itoa(int(step)),
	}
	if noExchange {
		env = append(env, pruneNoExchangeEnv+"=1")
	}
	if err := pruneChild(t, env...).Run(); err != nil {
		t.Fatalf("crashing child: %v", err)
	}
}

func (f pruneReplaceFixture) nextArchive() string {
	return f.next.Content.(evo.Extract).File.Path
}

func (f pruneReplaceFixture) recover(t *testing.T) (evo.RecoverResult, error) {
	t.Helper()
	return contractRunRecover(t, f.next, f.expected)
}

func TestPrune_RecoverSettlesEveryCrashOfReplace(t *testing.T) {
	oldTree := map[string]string{"index.js": "old", "lib/x.js": "x"}
	newTree := map[string]string{"index.js": "new"}
	for _, tc := range []struct {
		name       string
		step       publish.Step
		noExchange bool
		want       evo.RecoverState
		tree       map[string]string
	}{
		{"before the swap", publish.StepLocked, false, evo.RecoverIntact, oldTree},
		{"after the swap, before cleanup", publish.StepSwapped, false, evo.RecoverCompletedReplacement, newTree},
		{"between the two renames without exchange", publish.StepAside, true, evo.RecoverRestoredOriginal, oldTree},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPruneReplaceFixture(t)
			f.crashReplace(t, tc.step, tc.noExchange)
			if leftovers, err := publish.Leftovers(f.dest); err != nil || len(leftovers) == 0 {
				t.Fatalf("crash left %v (%v); want a leftover to recover", leftovers, err)
			}
			got, err := f.recover(t)
			if err != nil || got.State != tc.want || len(got.Leftovers) != 0 {
				t.Fatalf("Recover = %+v, %v; want state %v with nothing kept", got, err, tc.want)
			}
			if files := onDisk(t, f.dest); !equalFiles(files, tc.tree) {
				t.Fatalf("destination = %v, want the whole tree %v", files, tc.tree)
			}
			if names := advNames(t, f.parent); !slices.Equal(names, []string{"pkg"}) {
				t.Fatalf("Recover left %v in the parent", names)
			}
		})
	}
}

func TestPrune_RecoverOfAnUninterruptedTreeIsIntact(t *testing.T) {
	f := newPruneReplaceFixture(t)
	got, err := f.recover(t)
	if err != nil || got.State != evo.RecoverIntact || len(got.Leftovers) != 0 {
		t.Fatalf("Recover = %+v, %v; want Intact", got, err)
	}
}

func TestPrune_RecoverPreservesAnEditedDestinationAndItsLeftovers(t *testing.T) {
	f := newPruneReplaceFixture(t)
	f.crashReplace(t, publish.StepSwapped, false)
	plant(t, f.dest, map[string]string{"index.js": "edited after the crash"})
	got, err := f.recover(t)
	if !errors.Is(err, evo.ErrTreeChanged) || got.State != evo.RecoverUnrecoverable || len(got.Leftovers) != 1 {
		t.Fatalf("Recover = %+v, %v; want Unrecoverable (ErrTreeChanged) keeping the original", got, err)
	}
	if onDisk(t, f.dest)["index.js"] != "edited after the crash" {
		t.Fatal("Recover overwrote an edit it could not account for")
	}
	if sum := pruneChecksum(t, got.Leftovers[0]); sum != f.expected {
		t.Fatalf("kept leftover digests to %s, want the original %s", sum, f.expected)
	}
}

func TestPrune_RecoverNeverInfersTheOriginalFromLeftoversAlone(t *testing.T) {
	f := newPruneReplaceFixture(t)
	f.crashReplace(t, publish.StepAside, true)
	got, err := contractRunRecover(t, f.next, "sha256:not-what-was-planned")
	if !errors.Is(err, evo.ErrTreeChanged) || got.State != evo.RecoverUnrecoverable || len(got.Leftovers) != 2 {
		t.Fatalf("Recover = %+v, %v; want Unrecoverable keeping both leftovers", got, err)
	}
	if _, err := os.Lstat(f.dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Recover published a leftover it could not identify: %v", err)
	}
}

func contractRunRecover(t *testing.T, tree evo.Tree, expected string) (evo.RecoverResult, error) {
	t.Helper()
	var got evo.RecoverResult
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		var err error
		got, err = tree.Recover(ctx, expected)
		return err
	})
	return got, err
}
