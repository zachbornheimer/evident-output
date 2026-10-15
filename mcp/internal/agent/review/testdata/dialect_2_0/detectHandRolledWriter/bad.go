package fixture

import (
	"strings"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// livePhase re-implements Task.Writer: each write becomes the Doing text.
type livePhase struct{ task *evo.Task }

func (w *livePhase) Write(p []byte) (int, error) {
	w.task.Doing("%s", strings.TrimSpace(string(p)))
	return len(p), nil
}
