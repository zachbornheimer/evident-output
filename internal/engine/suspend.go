package engine

// Suspend opens an exclusive-TTY window (Confirm's prompt/answer, an external
// interactive program) around fn. It clears the live region and keeps every
// paint off the surface until fn returns, so a sibling's frame cannot
// overwrite the window; it does not hold the scheduler, so eligible siblings
// keep running (contract §3) and their rows paint once the window closes. It
// is unexported by contract: a stopped spinner is not a product state. Child
// processes write to task.Writer() so the live row keeps moving.
func (o *Output) Suspend(fn func() error) error {
	if fn == nil {
		return nil
	}
	window := o.openLiveWindow()

	err := fn()

	o.closeLiveWindow(window)
	return err
}

// liveWindow is one Suspend's hold on the live region: whether it quiesced
// the region at all (an interactive surface), and whether that cleared a
// painted frame the close must bring back.
type liveWindow struct {
	opened  bool
	cleared bool
}

// openLiveWindow quiesces the live region: no paint reaches the surface
// until every open window closes.
func (o *Output) openLiveWindow() liveWindow {
	o.mu.Lock()
	defer o.mu.Unlock()
	live := o.liveLocked()
	if live == nil || !live.IsInteractive() {
		return liveWindow{}
	}
	if o.live == nil {
		o.live = &liveEngine{surface: live}
	}
	o.live.quiesced++
	return liveWindow{opened: true, cleared: o.live.clear(live)}
}

// closeLiveWindow ends w. Once the last window closes, the region repaints:
// the frame w cleared, or the rows siblings started inside it.
func (o *Output) closeLiveWindow(w liveWindow) {
	if !w.opened {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.live == nil {
		return
	}
	o.live.quiesced--
	if o.live.quiesced > 0 {
		return
	}
	switch {
	case w.cleared && o.needsSpinnerAnimLocked():
		o.live.visible = true
		o.renderLiveLocked(true)
	case o.hasLiveActivityLocked():
		o.signalLiveLocked(true)
	}
}
