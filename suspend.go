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
	}
	o.mu.Unlock()

	err := fn()

	o.mu.Lock()
	if live != nil && wasActive && o.hasLiveActivityLocked() {
		o.live.visible = true
		o.renderLiveLocked(true)
	}
	o.mu.Unlock()
	return err
}
