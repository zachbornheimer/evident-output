package cacherootest_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/publish/cacheroot"
	"github.com/zachbornheimer/evident-output/internal/publish/cacheroot/cacherootest"
)

const exitingChildEnv = "EVO_CACHEROOTEST_EXITING_CHILD"

func TestMain(m *testing.M) { os.Exit(cacherootest.Run(m)) }

func scratchDirs(t *testing.T, tmp string) []string {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join(tmp, cacherootest.ScratchPrefix+"*"))
	if err != nil {
		t.Fatalf("glob scratch dirs in %s: %v", tmp, err)
	}
	return dirs
}

// A child test binary that exits through os.Exit inherits the parent's
// root: it creates no scratch directory of its own, so nothing is left
// behind once the parent cleans up.
func TestExitingChildLeavesNoScratchDir(t *testing.T) {
	if os.Getenv(exitingChildEnv) != "" {
		os.Exit(0)
	}
	tmp := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestExitingChildLeavesNoScratchDir$")
	cmd.Env = append(os.Environ(), exitingChildEnv+"=1", "TMPDIR="+tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("exiting child: %v\n%s", err, out)
	}
	if left := scratchDirs(t, tmp); len(left) != 0 {
		t.Fatalf("child left scratch dirs behind: %v", left)
	}
}

// The top-level owner removes its scratch root even when tests fail.
func TestOwnerRemovesScratchAfterFailure(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv(cacheroot.EnvVar, "")
	const failingCode = 1
	var during string
	code := cacherootest.Wrap(func() int {
		during = os.Getenv(cacheroot.EnvVar)
		return failingCode
	})
	if code != failingCode {
		t.Fatalf("Wrap returned %d, want %d", code, failingCode)
	}
	if filepath.Dir(during) != filepath.Clean(tmp) {
		t.Fatalf("scratch root %q is not under %q", during, tmp)
	}
	if left := scratchDirs(t, tmp); len(left) != 0 {
		t.Fatalf("owner left scratch dirs behind: %v", left)
	}
}
