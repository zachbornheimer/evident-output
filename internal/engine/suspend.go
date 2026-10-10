package engine

// Suspend opens an exclusive-TTY window (Confirm's prompt/answer, an external
// interactive program) around fn. It hides the live region so a sibling's
// frame cannot overwrite the window and repaints it once fn returns; it does
// not hold the scheduler, so eligible siblings keep running (contract §3).
// It is unexported by contract: a stopped spinner is not a product state.
// Child processes write to task.Writer() so the live row keeps moving.
func (o *Output) Suspend(fn func() error) error {
	if fn == nil {
		return nil
	}
	o.mu.Lock()
	live := o.liveLocked()
	wasActive := live != nil && o.live != nil && o.live.clear(live)
	if wasActive {
		// Confirm's prompt/answer must not be overwritten by a sibling
		// task's live frame. Restored below once fn returns.
		o.live.visible = false
	}
	o.mu.Unlock()

	err := fn()

	o.mu.Lock()
	if wasActive && o.needsSpinnerAnimLocked() {
		o.live.visible = true
		o.renderLiveLocked(true)
	}
	o.mu.Unlock()
	return err
}
