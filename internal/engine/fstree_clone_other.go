//go:build !darwin && !linux

package engine

import (
	"errors"
	"os"
)

// cloneEntry and reflinkFile have no copy-on-write here; Clone copies
// bytes.
func cloneEntry(string, string) error { return errCloneUnsupported }

func reflinkFile(*os.File, *os.File) error { return errCloneUnsupported }

var errCloneUnsupported = errors.New("copy-on-write clone unsupported")
