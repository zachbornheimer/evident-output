package tree_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/publish"
)

// nestedFixture is a parent tree with a child tree inside it.
type nestedFixture struct {
	parent, child string
}

func newNestedFixture(t *testing.T) nestedFixture {
	t.Helper()
	parent := filepath.Join(t.TempDir(), "node_modules")
	plant(t, parent, map[string]string{"a.js": "parent old", "pkg/node_modules/dep/index.js": "dep old"})
	return nestedFixture{parent: parent, child: filepath.Join(parent, "pkg", "node_modules", "dep")}
}

// A child's unfinished stage is not part of its parent: the parent's
// Checksum is the same with or without it.
func TestPrune_ChildStagingIsNotPartOfTheParentChecksum(t *testing.T) {
	f := newNestedFixture(t)
	before := pruneChecksum(t, f.parent)
	staged, err := publish.StageTree(context.Background(), f.child, 0, func(_ context.Context, root string) error {
		plant(t, root, map[string]string{"index.js": "dep new"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = staged.Discard() }()
	if got := pruneChecksum(t, f.parent); got != before {
		t.Fatalf("parent Checksum with a child staging = %s, want %s", got, before)
	}
}

// A parent commit lands between the child Replace's staging and its swap,
// carrying the child's stage away with the old parent. That can happen
// only when stages fall back to beside their destination (the staging
// root on another volume). The child Replace is ErrTreeChanged, never a
// bare rename error, and leaves nothing behind.
func TestPrune_ReplaceWhoseStageAParentSwapCarriedAwayIsTreeChanged(t *testing.T) {
	f := newNestedFixture(t)
	childExpected := pruneChecksum(t, f.child)
	newParent := map[string]string{"a.js": "parent new", "pkg/node_modules/dep/index.js": "dep old"}
	defer publish.InjectFaults(publish.Faults{StageBeside: true, At: func(step publish.Step, at string) {
		if step != publish.StepStaged || at != f.child {
			return
		}
		staged, err := publish.StageTree(context.Background(), f.parent, 0, func(_ context.Context, root string) error {
			plant(t, root, newParent)
			return nil
		})
		if err == nil {
			err = staged.Commit(context.Background(), publish.Guard{})
		}
		if err != nil {
			t.Errorf("parent commit: %v", err)
		}
	}})()
	next := evo.Tree{Path: f.child, Content: evo.Extract{File: archive(t, t.TempDir(), "dep.tgz", map[string]string{"index.js": "dep new"})}}
	err := contractRun(t, evo.Config{}, func(ctx context.Context) error { return next.Replace(ctx, childExpected) })
	if !errors.Is(err, evo.ErrTreeChanged) {
		t.Fatalf("child Replace after its stage was carried away = %v, want ErrTreeChanged", err)
	}
	if got := onDisk(t, f.parent); !equalFiles(got, newParent) {
		t.Fatalf("parent = %v, want the parent commit's tree untouched", got)
	}
	if names := advNames(t, filepath.Dir(f.child)); !slices.Equal(names, []string{"dep"}) {
		t.Fatalf("child Replace left %v beside the child", names)
	}
}
