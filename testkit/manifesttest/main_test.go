package manifesttest_test

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/freshness"
	"github.com/zachbornheimer/evident-output/testkit/manifesttest"
)

// reportEnv names the file a child test binary writes its redirected cache
// directory into.
const reportEnv = "EVO_MANIFESTTEST_REPORT"

func TestMain(m *testing.M) { manifesttest.Main(m) }

func TestDefaultManifestsAreRedirectedToAFreshTemporaryDirectory(t *testing.T) {
	cacheDir, err := freshness.NewSystemManifestEnvironment().UserCacheDir()
	if err != nil {
		t.Fatalf("UserCacheDir: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(cacheDir), "evo-manifest-test-") {
		t.Fatalf("cache dir %q is not a manifesttest temporary directory", cacheDir)
	}
	if info, err := os.Stat(cacheDir); err != nil || !info.IsDir() {
		t.Fatalf("cache dir %q is not an existing directory: %v", cacheDir, err)
	}
}

// TestReportCacheDir is the child half of
// TestTheTemporaryDirectoryIsRemovedWhenTheRunEnds: it records the redirected
// cache directory so the parent can check it afterward.
func TestReportCacheDir(t *testing.T) {
	report := os.Getenv(reportEnv)
	if report == "" {
		t.Skip("runs only as a child of TestTheTemporaryDirectoryIsRemovedWhenTheRunEnds")
	}
	cacheDir, err := freshness.NewSystemManifestEnvironment().UserCacheDir()
	if err != nil {
		t.Fatalf("UserCacheDir: %v", err)
	}
	if err := os.WriteFile(report, []byte(cacheDir), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTheTemporaryDirectoryIsRemovedWhenTheRunEnds(t *testing.T) {
	report := filepath.Join(t.TempDir(), "cache-dir")
	child := exec.Command(os.Args[0], "-test.run=^TestReportCacheDir$")
	child.Env = append(os.Environ(), reportEnv+"="+report)
	if out, err := child.CombinedOutput(); err != nil {
		t.Fatalf("child test binary: %v\n%s", err, out)
	}
	recorded, err := os.ReadFile(report)
	if err != nil {
		t.Fatalf("child did not report its cache dir: %v", err)
	}
	if _, err := os.Stat(string(recorded)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("cache dir %q still exists after the child run ended: %v", recorded, err)
	}
}
