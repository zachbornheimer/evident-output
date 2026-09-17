//go:build linux

package engine

import (
	"os"
	"syscall"
	"testing"
	"time"
)

// ctimeOf reports path's inode change time — see ctime_darwin_test.go.
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
	return time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec), true
}
