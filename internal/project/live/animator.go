package live

// ensureAnimator starts the background tick that re-renders the region so
// indeterminate spinners advance without waiting for Progress calls, and
// that promotes visibility once VisibilityDelay elapses.
func (e *Engine) ensureAnimator() {
	if e.cfg.Scene.Ended() {
		return
	}
	// A running animator re-checks Animating on every tick and stops
	// itself, so skip that O(n) scan on each signal while it runs.
	e.animMu.Lock()
	running := e.animRunning
	e.animMu.Unlock()
	if running {
		return
	}
	switch {
	case e.waitingDelay:
		// Keep a ticker so the region paints when the delay elapses.
	case !e.visible:
		return
	case !e.cfg.Board.Animating() && !e.pendingRedraw.Load():
		e.stopAnimator()
		return
	}
	e.animMu.Lock()
	if e.animRunning {
		e.animMu.Unlock()
		return
	}
	stop := make(chan struct{})
	e.animStop = stop
	e.animRunning = true
	e.animMu.Unlock()
	go e.animate(stop)
}

func (e *Engine) stopAnimator() {
	e.animMu.Lock()
	if e.animRunning && e.animStop != nil {
		close(e.animStop)
		e.animStop = nil
		e.animRunning = false
	}
	e.animMu.Unlock()
}

// Animating reports whether the animator is running. It takes the
// animator's own lock, not the run's.
func (e *Engine) Animating() bool {
	if e == nil {
		return false
	}
	e.animMu.Lock()
	defer e.animMu.Unlock()
	return e.animRunning
}

// animate ticks on the wall clock: spinner cadence is independent of the
// domain clock and of Progress and Phase call rate. The domain clock still
// selects the glyph (a fixed clock freezes animation for golden tests).
func (e *Engine) animate(stop <-chan struct{}) {
	t := wall.NewTimer(untilNextSpinnerSlot(wall.Now()))
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			t.Reset(untilNextSpinnerSlot(wall.Now()))
			e.cfg.Lock.Lock()
			next := e.tick()
			if next == tickPaint || next == tickFlush {
				e.paintUnlocked()
			}
			e.cfg.Lock.Unlock()
			if next == tickStop || next == tickFlush {
				return
			}
		}
	}
}

// step is what one animator tick decided.
type step int

const (
	tickContinue step = iota // nothing to paint; keep ticking
	tickStop                 // nothing to paint; the animator has stopped
	tickPaint                // paint a frame, keep ticking
	tickFlush                // paint the last coalesced change; the animator has stopped
)

// tick decides one tick: it settles visibility and stops the animator when
// nothing is left to animate, and skips a glyph-only repaint while an
// overloaded surface was written too recently to need it.
func (e *Engine) tick() step {
	if e.cfg.Scene.Ended() {
		e.stopAnimator()
		return tickStop
	}
	if e.quiesced > 0 {
		return tickContinue
	}
	if e.waitingDelay && e.cfg.Board.Active() {
		delay := e.cfg.VisibilityDelay
		if delay <= 0 || e.cfg.Clock.Now().Sub(e.activitySince) >= delay {
			e.waitingDelay = false
			e.Show()
		}
		return tickContinue
	}
	if !e.visible {
		e.stopAnimator()
		return tickStop
	}
	if !e.cfg.Board.Animating() {
		e.stopAnimator()
		// Paint the last coalesced change before going quiet, so the
		// frame on screen is never older than the run.
		if e.pendingRedraw.Load() {
			return tickFlush
		}
		return tickStop
	}
	if !e.pendingRedraw.Load() && e.shouldSkipGlyph() {
		e.skippedGlyph.Add(1)
		return tickContinue
	}
	return tickPaint
}
