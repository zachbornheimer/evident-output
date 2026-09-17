//go:build windows

package manifest

import "time"

// windowsLockPollInterval bounds how often acquireLock retries exclusive
// file creation on Windows (lock_windows.go's best-effort fallback).
const windowsLockPollInterval = 10 * time.Millisecond

// pollTicker is the facade acquireLock polls through instead of calling
// time.NewTicker directly.
type pollTicker struct{ t *time.Ticker }

func newPollTicker() pollTicker          { return pollTicker{t: time.NewTicker(windowsLockPollInterval)} }
func (p pollTicker) c() <-chan time.Time { return p.t.C }
func (p pollTicker) stop()               { p.t.Stop() }
