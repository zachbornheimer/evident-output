//go:build !darwin && !linux

package fs

import "errors"

// ErrExchangeUnsupported is what Exchange returns where the OS has no atomic
// swap; callers fall back to moving the old entry aside first.
var ErrExchangeUnsupported = errors.New("fs: atomic exchange unsupported on this platform")

// Exchange has no atomic swap here.
func Exchange(string, string) error { return ErrExchangeUnsupported }
