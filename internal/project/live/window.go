package live

// Window is one exclusive hold on the terminal (a Confirm prompt, an
// interactive child program): whether it quiesced the region at all, and
// whether that cleared a painted frame the close must bring back.
type Window struct {
	opened  bool
	cleared bool
}

// Open quiesces the region: no paint reaches the surface until every open
// window closes. A surface that is not interactive needs no window.
func (e *Engine) Open() Window {
	if !e.Interactive() {
		return Window{}
	}
	e.quiesced++
	return Window{opened: true, cleared: e.Clear()}
}

// Close ends w. Once the last window closes, the durable text held while
// windows were open goes to emit, in arrival order, above the region; then
// the region repaints: the frame w cleared, or the rows that started inside
// it.
func (e *Engine) Close(w Window, emit func(text string)) {
	if !w.opened {
		return
	}
	e.quiesced--
	if e.quiesced > 0 {
		return
	}
	e.Flush(emit)
	switch {
	case w.cleared && e.cfg.Board.Animating():
		e.Show()
	case e.cfg.Board.Active():
		e.Signal(true)
	}
}

// Hold keeps text for the close of the last open window and reports
// whether it did: while a window owns the terminal a sibling's row must not
// land inside it.
func (e *Engine) Hold(text string) bool {
	if e == nil || e.quiesced == 0 {
		return false
	}
	e.held = append(e.held, text)
	return true
}

// Flush hands every held text to emit, in arrival order.
func (e *Engine) Flush(emit func(text string)) {
	if e == nil {
		return
	}
	held := e.held
	e.held = nil
	for _, text := range held {
		emit(text)
	}
}
