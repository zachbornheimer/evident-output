package evo

// Suspend temporarily pauses interactive presentation for host-owned output.
// In v0.3 plain/non-interactive mode this is a no-op around fn.
// With a live surface it clears the live region, runs fn, then resumes.
func (o *Output) Suspend(fn func() error) error {
	if fn == nil {
		return nil
	}
	o.mu.Lock()
	live := o.liveLocked()
	wasActive := o.live != nil && o.live.liveActive
	if live != nil && wasActive {
		live.ClearLive()
		o.live.liveActive = false
		// Suspend means suspend: while fn runs (e.g. Confirm's own durable
		// prompt write), no redraw may repaint the live region even if fn
		// itself writes a durable line — "no spinner while waiting on a
		// human" holds for the whole quiesce window, not just its first
		// clear. Restored below, symmetrically, once fn returns.
		o.live.visible = false
	}
	o.mu.Unlock()

	err := fn()

	o.mu.Lock()
	// Resume painting only when something is actually Running. A settled Done
	// task with determinate progress still counts as "live activity" for
	// VisibilityDelay purposes (hasLiveActivityLocked), but repainting that
	// static state after Suspend produces a stray live frame with nothing in
	// motion (evo-rec.md "E" — post-Suspend spinner frame with nothing Running).
	if live != nil && wasActive && o.needsSpinnerAnimLocked() {
		o.live.visible = true
		o.renderLiveLocked(true)
	}
	o.mu.Unlock()
	return err
}
