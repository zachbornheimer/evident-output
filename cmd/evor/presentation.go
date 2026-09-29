package main

import (
	"fmt"
	"time"
)

const (
	modeAuto   = "auto"
	modeTTY    = "tty"
	modeStream = "stream"
)

func resolvePresentation(requested string, stdoutIsTerminal bool) (string, error) {
	switch requested {
	case "", modeAuto:
		if stdoutIsTerminal {
			return modeTTY, nil
		}
		return modeStream, nil
	case modeTTY, modeStream:
		return requested, nil
	default:
		return "", fmt.Errorf("mode must be %q, %q, or %q", modeAuto, modeTTY, modeStream)
	}
}

func streamMode(mode string) bool {
	return mode == modeStream
}

func nextHeartbeatWait(now, last time.Time, interval time.Duration) time.Duration {
	wait := last.Add(interval).Sub(now)
	if wait < 0 {
		return 0
	}
	return wait
}
