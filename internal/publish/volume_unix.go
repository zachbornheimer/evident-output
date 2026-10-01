//go:build unix

package publish

import (
	"os"
	"syscall"
)

// sameVolume reports whether a and b live on one filesystem, so a rename
// between them is atomic.
func sameVolume(a, b string) bool {
	ai, errA := os.Stat(a)
	bi, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return false
	}
	as, okA := ai.Sys().(*syscall.Stat_t)
	bs, okB := bi.Sys().(*syscall.Stat_t)
	return okA && okB && as.Dev == bs.Dev
}
