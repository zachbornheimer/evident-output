package checksum

import (
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/publish/stagename"
)

// A writer staging a child tree puts its unfinished stage inside the
// parent; the parent's digest must not see it, as a directory or a file,
// at any depth.
func TestTreeLeavesOutInFlightStaging(t *testing.T) {
	root := t.TempDir()
	plant(t, root, map[string]string{"pkg/index.js": "x", "sub/keep.js": "y"})
	before := mustTree(t, Engine{}, root)
	stage := filepath.Base(stagename.New(filepath.Join(root, "pkg")))
	plant(t, root, map[string]string{stage + "/index.js": "half-written", "sub/" + stage: "staged file"})
	if got := mustTree(t, Engine{}, root); got != before {
		t.Fatalf("digest with in-flight staging = %s, want %s (staging left out)", got, before)
	}
}

// Only the exact staging scheme is left out: a real entry with a similar
// name is content.
func TestTreeCountsLookalikeStagingNames(t *testing.T) {
	for _, name := range []string{
		".evo-foo.tmp",
		".evo-0123abcd-short.tmp",
		".evo-0123abcd-ABCDEFGHIJKLMNOPQRSTUVWXYZ.tmp",
		".evo-0123abcd-abcdefghijklmnopqrstuvwxyz.tmp.bak",
		".evo-0123ABCD-abcdefghijklmnopqrstuvwxyz.tmp",
		".evo-0123abcd-abcdefghijklmnopqrstuvwxy1.tmp",
		"evo-0123abcd-abcdefghijklmnopqrstuvwxyz.tmp",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			plant(t, root, map[string]string{"index.js": "x"})
			before := mustTree(t, Engine{}, root)
			plant(t, root, map[string]string{name: "real content"})
			if mustTree(t, Engine{}, root) == before {
				t.Fatalf("%s was left out of the digest; only exact staging names are", name)
			}
		})
	}
}

// The scheme itself: a name New produces is staging.
func TestStagingSchemeRoundTrips(t *testing.T) {
	if name := filepath.Base(stagename.New("/x/pkg")); !stagename.Is(name) {
		t.Fatalf("Is(%q) = false for a name New produced", name)
	}
}
