//go:build windows

package fs

import (
	"time"

	"github.com/zachbornheimer/evident-output/internal/clock"
)

// windowsLockPollInterval bounds how often AcquireFileLock retries exclusive
// file creation on Windows (lock_windows.go's best-effort fallback).
const windowsLockPollInterval = 10 * time.Millisecond

// pollTicker is the facade AcquireFileLock polls through instead of calling
// time.NewTicker directly.
type pollTicker struct{ t *time.Ticker }

func newPollTicker() pollTicker {
	return pollTicker{t: clock.System().NewTicker(windowsLockPollInterval)}
}
func (p pollTicker) c() <-chan time.Time { return p.t.C }
func (p pollTicker) stop()               { p.t.Stop() }
