package graph

// StackMarks is what one walk of a goroutine's stack reports: how many task
// callbacks it is inside (runCallback), how many resource claims it holds
// (Holds.Enter) and how many container builders it is running (runBuilder).
type StackMarks struct {
	Callbacks int
	Claims    int
	Builders  int
}

// callbackFrames marks runCallback; builderFrames marks runBuilder.
var (
	callbackFrames FrameMarker
	builderFrames  FrameMarker
)

// ReadStackMarks counts every mark in a single walk, and walks nothing when
// no marked function has run yet.
func ReadStackMarks() StackMarks {
	callback, holding, builder := callbackFrames.Name(), processHolds.Frame(), builderFrames.Name()
	var m StackMarks
	if callback == nil && holding == nil && builder == nil {
		return m
	}
	WalkStack(func(function string) {
		switch {
		case callback != nil && function == *callback:
			m.Callbacks++
		case holding != nil && function == *holding:
			m.Claims++
		case builder != nil && function == *builder:
			m.Builders++
		}
	})
	return m
}

// CallbackDepth is how many task callbacks the calling goroutine is inside:
// zero for a plain caller, one for a callback, more when a waiter donated its
// goroutine to nested work before parking.
func CallbackDepth() int { return ReadStackMarks().Callbacks }

// WaiterStack reads a waiting goroutine's stack marks at most once, and only
// when an answer needs them: a Wait on already-settled work while no claim is
// held anywhere in the process walks nothing. The zero value reads on demand.
type WaiterStack struct {
	read        bool
	marks       StackMarks
	creatorRead bool
	creator     GoroutineID
}

// UnmarkedStack is a WaiterStack already known to be inside no callback and
// to hold no claim, so it never walks.
func UnmarkedStack() *WaiterStack { return &WaiterStack{read: true} }

func (w *WaiterStack) load() StackMarks {
	if !w.read {
		w.marks = ReadStackMarks()
		w.read = true
	}
	return w.marks
}

// HoldsClaim reports whether the waiting goroutine holds a resource claim,
// on its own stack or through the goroutine that started it. A claim held
// further up a chain of goroutines is not seen.
func (w *WaiterStack) HoldsClaim() bool {
	if !processHolds.Any() {
		return false
	}
	if w.load().Claims > 0 {
		return true
	}
	return processHolds.Blocked(w.Creator())
}

// Creator is the goroutine that started the waiting goroutine (zero for one
// the runtime started), read once: it needs the whole traceback.
func (w *WaiterStack) Creator() GoroutineID {
	if !w.creatorRead {
		_, w.creator = CurrentGoroutineLineage()
		w.creatorRead = true
	}
	return w.creator
}

// InsideBuilder reports whether the waiting goroutine is running a container
// builder.
func (w *WaiterStack) InsideBuilder() bool { return w.load().Builders > 0 }

// CallbackDepth is how many task callbacks the waiting goroutine is inside.
func (w *WaiterStack) CallbackDepth() int { return w.load().Callbacks }
