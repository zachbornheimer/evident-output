package architecture

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestImportingRootPackagePerformsNoIOOrGoroutines proves the root evo
// package is safe to import for its side effects alone: no goroutine
// starts and no file is written before main() runs. It builds two
// otherwise-identical programs (see importprobe/candidate and
// importprobe/baseline) that differ only in whether they import evo, runs
// each with an isolated, empty HOME/XDG cache tree, and diffs the results
// — a real subprocess boundary, not a same-process approximation, because
// package-level var initializers and any hidden init() only run once per
// process and this repo's own tests already share a process with evo
// imported.
func TestImportingRootPackagePerformsNoIOOrGoroutines(t *testing.T) {
	root := moduleRoot(t)
	binDir := t.TempDir()

	candidateBin := buildProbe(t, root, binDir, "candidate")
	baselineBin := buildProbe(t, root, binDir, "baseline")

	candidate := runProbe(t, candidateBin)
	baseline := runProbe(t, baselineBin)

	if candidate.goroutines != baseline.goroutines {
		t.Fatalf("importing evo changed goroutine count at process start: baseline=%d candidate=%d",
			baseline.goroutines, candidate.goroutines)
	}
	if len(candidate.filesWritten) > 0 {
		t.Fatalf("importing evo wrote files before main() ran: %v", candidate.filesWritten)
	}
}

// buildProbe compiles internal/architecture/importprobe/<name> into binDir
// and returns the built binary's path.
func buildProbe(t *testing.T, root, binDir, name string) string {
	t.Helper()
	binPath := filepath.Join(binDir, name)
	pkgPath := "./internal/architecture/importprobe/" + name
	cmd := exec.Command("go", "build", "-o", binPath, pkgPath)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkgPath, err, out)
	}
	return binPath
}

// probeResult is one import-probe program's reported state: its own
// runtime.NumGoroutine() at process start, and every file that appeared
// under its isolated HOME/XDG cache tree by the time it exited.
type probeResult struct {
	goroutines   int
	filesWritten []string
}

// runProbe executes bin with an isolated, empty home/cache tree (so any
// file the process writes is easy to find) and parses its single line of
// stdout as a goroutine count.
func runProbe(t *testing.T, bin string) probeResult {
	t.Helper()
	home := t.TempDir()

	cmd := exec.Command(bin)
	cmd.Env = []string{
		"HOME=" + home,
		"XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, "config"),
		"XDG_STATE_HOME=" + filepath.Join(home, "state"),
		"TMPDIR=" + home,
		"PATH=" + os.Getenv("PATH"),
	}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run %s: %v", bin, err)
	}

	return probeResult{
		goroutines:   parseGoroutineCount(t, out),
		filesWritten: filesUnder(t, home),
	}
}

// parseGoroutineCount parses the probe program's single line of stdout
// (fmt.Println(runtime.NumGoroutine())) as an int.
func parseGoroutineCount(t *testing.T, stdout []byte) int {
	t.Helper()
	line := strings.TrimSpace(string(stdout))
	n, err := strconv.Atoi(line)
	if err != nil {
		t.Fatalf("parse goroutine count from probe stdout %q: %v", line, err)
	}
	return n
}

// filesWritten reports every regular file that exists under dir — used to
// detect any write into the probe's isolated home/cache tree, since a
// freshly created t.TempDir() starts empty.
func filesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return found
}
