package engine

import "github.com/zachbornheimer/evident-output/internal/graph"

// stackMarks is what one walk of a goroutine's stack reports: how many task
// callbacks it is inside (runCallback) and how many resource claims it
// holds (runHoldingResource).
type stackMarks struct {
	callbacks int
	claims    int
	builders  int
}

// readStackMarks counts both marks in a single walk, and walks nothing when
// neither marked function has run yet.
func readStackMarks() stackMarks {
	callback, holding, builder := callbackFrames.Name(), graph.ProcessHolds().Frame(), builderFrames.Name()
	var m stackMarks
	if callback == nil && holding == nil && builder == nil {
		return m
	}
	graph.WalkStack(func(function string) {
		switch {
		case callback != nil && function == *callback:
			m.callbacks++
		case holding != nil && function == *holding:
			m.claims++
		case builder != nil && function == *builder:
			m.builders++
		}
	})
	return m
}

// waiterStack reads a waiting goroutine's stack marks at most once, and
// only when an answer needs them: a Wait on already-settled work while no
// claim is held anywhere in the process walks nothing.
type waiterStack struct {
	read  bool
	marks stackMarks
}

func (w *waiterStack) load() stackMarks {
	if !w.read {
		w.marks = readStackMarks()
		w.read = true
	}
	return w.marks
}

// holdsClaim reports whether the waiting goroutine holds a resource claim,
// on its own stack or through the goroutine that started it. A claim held
// further up a chain of goroutines is not seen.
func (w *waiterStack) holdsClaim() bool {
	if !graph.ProcessHolds().Any() {
		return false
	}
	if w.load().claims > 0 {
		return true
	}
	_, creator := graph.CurrentGoroutineLineage()
	return graph.ProcessHolds().Blocked(creator)
}

// callbackDepth is how many task callbacks the waiting goroutine is inside.
func (w *waiterStack) callbackDepth() int { return w.load().callbacks }
