//go:build darwin

package engine

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// ctimeOf reports path's inode change time — the only portable-enough
// signal that a chmod syscall ran even when it happened to set the mode to
// its already-current value (unlike ModTime, which chmod never touches).
func ctimeOf(t *testing.T, path string) (time.Time, bool) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	return time.Unix(stat.Ctimespec.Sec, stat.Ctimespec.Nsec), true
}
