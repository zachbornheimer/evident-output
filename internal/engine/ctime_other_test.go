//go:build !darwin && !linux

package engine

import (
	"testing"
	"time"
)

// ctimeOf has no portable implementation on this platform — the caller
// skips the assertion that depends on it. See ctime_darwin_test.go/
// ctime_linux_test.go for the platforms this is proven on.
func ctimeOf(t *testing.T, path string) (time.Time, bool) {
	t.Helper()
	return time.Time{}, false
}
