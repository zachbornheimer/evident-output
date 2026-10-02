//go:build !darwin && !linux

package publish

import "errors"

// exchange has no atomic swap here; commitTree falls back to moving the
// old tree aside first.
func exchange(string, string) error { return errExchangeUnsupported }

var errExchangeUnsupported = errors.New("publish: atomic exchange unsupported on this platform")
