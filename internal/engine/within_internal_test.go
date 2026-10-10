package engine

import "time"

// within runs f on its own goroutine and reports whether it returned before d.
func within(d time.Duration, f func() error) (error, bool) {
	done := make(chan error, 1)
	go func() { done <- f() }()
	select {
	case err := <-done:
		return err, true
	case <-time.After(d):
		return nil, false
	}
}
