//go:build unix

package terminal

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// resizeWatch is the SIGWINCH subscription for a single ANSI driver.
type resizeWatch struct {
	mu       sync.Mutex
	stop     chan struct{}
	running  bool
	onResize func()
}

// StartResizeWatch listens for SIGWINCH, refreshes size from sizeFile, and
// invokes onResize (typically a live redraw). No-op without sizeFile.
// Safe to call more than once; subsequent calls replace the callback only if
// a watch is already running.
func (a *ANSI) StartResizeWatch(onResize func()) {
	if a == nil {
		return
	}
	a.mu.Lock()
	if a.sizeFile == nil {
		a.mu.Unlock()
		return
	}
	if a.resize == nil {
		a.resize = &resizeWatch{}
	}
	rw := a.resize
	a.mu.Unlock()

	rw.mu.Lock()
	rw.onResize = onResize
	if rw.running {
		rw.mu.Unlock()
		return
	}
	rw.stop = make(chan struct{})
	rw.running = true
	stop := rw.stop
	rw.mu.Unlock()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	go func() {
		defer signal.Stop(ch)
		for {
			select {
			case <-stop:
				return
			case <-ch:
				a.RefreshSize()
				rw.mu.Lock()
				cb := rw.onResize
				rw.mu.Unlock()
				if cb != nil {
					cb()
				}
			}
		}
	}()
}

// StopResizeWatch ends the SIGWINCH goroutine. Safe to call when not running.
func (a *ANSI) StopResizeWatch() {
	if a == nil {
		return
	}
	a.mu.Lock()
	rw := a.resize
	a.mu.Unlock()
	if rw == nil {
		return
	}
	rw.mu.Lock()
	if rw.running && rw.stop != nil {
		close(rw.stop)
		rw.stop = nil
		rw.running = false
	}
	rw.mu.Unlock()
}
