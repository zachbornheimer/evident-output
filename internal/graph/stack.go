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
