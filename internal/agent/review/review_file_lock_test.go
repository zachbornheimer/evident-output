package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// API-055 (ZYS-831): the detector must catch lock files and flock regions
// around evo.File/Files/Patch/Effect, not only sync.Mutex.

const lockFileSrcHeader = `package p
import (
  "context"
  "os"
  "syscall"
  "golang.org/x/sys/unix"
  "github.com/gofrs/flock"
  evo "github.com/zachbornheimer/evident-output"
)
var _ = os.Getpid
var _ = syscall.Getpid
var _ = unix.Getpid
var _ = flock.New
`

func lockFileSrc(body string) string { return lockFileSrcHeader + body }

func api055Findings(res review.Result) []review.Finding {
	var out []review.Finding
	for _, f := range res.Findings {
		if f.RuleID == "API-055" {
			out = append(out, f)
		}
	}
	return out
}

func lineContaining(t *testing.T, src, marker string) int {
	t.Helper()
	for i, line := range strings.Split(src, "\n") {
		if strings.Contains(line, marker) {
			return i + 1
		}
	}
	t.Fatalf("marker %q not in source", marker)
	return 0
}

func TestAPI055_LockFilesAroundEvoCalls_FireAtAcquisition(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		acquire  string
		wantText string
	}{
		{"syscall flock deferred unlock around File", `
func f(ctx context.Context, fd int, p string) error {
  syscall.Flock(fd, syscall.LOCK_EX)
  defer syscall.Flock(fd, syscall.LOCK_UN)
  return evo.File(ctx, evo.FileSpec{Path: p})
}
`, "syscall.Flock(fd, syscall.LOCK_EX)", "syscall.Flock(fd, syscall.LOCK_UN)"},
		{"syscall flock bare unlock around File", `
func f(ctx context.Context, fd int, p string) error {
  if err := syscall.Flock(fd, syscall.LOCK_SH); err != nil {
    return err
  }
  err := evo.File(ctx, evo.FileSpec{Path: p})
  syscall.Flock(fd, syscall.LOCK_UN)
  return err
}
`, "syscall.Flock(fd, syscall.LOCK_SH)", "syscall.Flock(fd, syscall.LOCK_UN)"},
		{"unix flock closed handle around Effect", `
func f(ctx context.Context, f *os.File) error {
  unix.Flock(int(f.Fd()), unix.LOCK_EX)
  defer f.Close()
  return evo.Effect(ctx, evo.EffectSpec{Verb: evo.EffectUpdate, Object: "x", Quantity: 1}, func(context.Context) error { return nil })
}
`, "unix.Flock(int(f.Fd()), unix.LOCK_EX)", "unix.Flock"},
		{"O_EXCL lock file removed around Patch", `
func f(ctx context.Context, p string) error {
  lf, err := os.OpenFile(p+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
  if err != nil {
    return err
  }
  defer lf.Close()
  defer os.Remove(p + ".lock")
  return evo.Patch(ctx, evo.PatchSpec{Path: p})
}
`, "os.OpenFile(p+\".lock\"", "os.Remove(p + \".lock\")"},
		{"O_EXCL lock file bare remove around Files", `
func f(ctx context.Context, p string) error {
  lf, err := os.OpenFile(p+".lock", os.O_CREATE|os.O_EXCL, 0o600)
  if err != nil {
    return err
  }
  lf.Close()
  err = evo.Files(ctx, nil)
  os.Remove(p + ".lock")
  return err
}
`, "os.OpenFile(p+\".lock\"", "os.Remove(p + \".lock\")"},
		{"gofrs flock deferred unlock around File", `
func f(ctx context.Context, p string) error {
  fl := flock.New(p + ".lock")
  fl.Lock()
  defer fl.Unlock()
  return evo.File(ctx, evo.FileSpec{Path: p})
}
`, "fl.Lock()", "fl.Unlock()"},
		{"gofrs TryLock around File", `
func f(ctx context.Context, p string) error {
  fl := flock.New(p + ".lock")
  ok, err := fl.TryLock()
  if err != nil || !ok {
    return err
  }
  defer fl.Unlock()
  return evo.File(ctx, evo.FileSpec{Path: p})
}
`, "fl.TryLock()", "fl.Unlock()"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := lockFileSrc(tc.body)
			got := api055Findings(review.GoSource("lock.go", src))
			if len(got) != 1 {
				t.Fatalf("want exactly one API-055 finding, got %d: %+v", len(got), got)
			}
			if want := lineContaining(t, src, tc.acquire); got[0].Line != want {
				t.Fatalf("finding line = %d, want acquisition line %d", got[0].Line, want)
			}
			if !strings.Contains(got[0].Suggestion, tc.wantText) {
				t.Fatalf("suggestion %q does not name the release %q", got[0].Suggestion, tc.wantText)
			}
			if !strings.Contains(got[0].Suggestion, "drop") {
				t.Fatalf("a lock guarding only the evo call must say drop: %q", got[0].Suggestion)
			}
		})
	}
}

func TestAPI055_LockFileRegionWithOtherWork_SaysKeepAndNarrow(t *testing.T) {
	src := lockFileSrc(`
func f(ctx context.Context, fd int, p string, seen map[string]int) error {
  syscall.Flock(fd, syscall.LOCK_EX)
  defer syscall.Flock(fd, syscall.LOCK_UN)
  seen[p]++
  return evo.File(ctx, evo.FileSpec{Path: p})
}
`)
	got := api055Findings(review.GoSource("lock.go", src))
	if len(got) != 1 {
		t.Fatalf("want one API-055 finding, got %+v", got)
	}
	if !strings.Contains(got[0].Suggestion, "keep") || strings.Contains(got[0].Suggestion, "drop") {
		t.Fatalf("a region with other work must say keep/narrow, not drop: %q", got[0].Suggestion)
	}
}

func TestAPI055_LockRegionsWithoutEvoManagedCall_StaySilent(t *testing.T) {
	cases := map[string]string{
		"flock read-modify-write with no evo call": `
func f(fd int, m map[string]int) {
  syscall.Flock(fd, syscall.LOCK_EX)
  defer syscall.Flock(fd, syscall.LOCK_UN)
  m["ports"]++
}
`,
		"O_EXCL lock with no evo call": `
func f(p string) error {
  lf, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL, 0o600)
  if err != nil {
    return err
  }
  defer lf.Close()
  defer os.Remove(p)
  return nil
}
`,
		"evo.File with no lock": `
func f(ctx context.Context, p string) error {
  return evo.File(ctx, evo.FileSpec{Path: p})
}
`,
		"O_EXCL OpenFile never removed": `
func f(ctx context.Context, p string) error {
  lf, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL, 0o600)
  if err != nil {
    return err
  }
  defer lf.Close()
  return evo.File(ctx, evo.FileSpec{Path: p})
}
`,
		"Flock and O_EXCL only in comments and strings": `
// syscall.Flock(fd, syscall.LOCK_EX) then evo.File; os.OpenFile(p, os.O_EXCL, 0)
func f(ctx context.Context, p string) error {
  _ = "syscall.Flock(fd, syscall.LOCK_EX); defer os.Remove(p)"
  return evo.File(ctx, evo.FileSpec{Path: p})
}
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if got := api055Findings(review.GoSource("lock.go", lockFileSrc(body))); len(got) != 0 {
				t.Fatalf("false positive API-055: %+v", got)
			}
		})
	}
}
