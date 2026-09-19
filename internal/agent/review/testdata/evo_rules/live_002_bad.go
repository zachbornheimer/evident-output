// Fixture: EVO-LIVE-002 must fire. time.NewTicker pokes Doing to keep
// the UI alive.
package live002

import (
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

func keepAlive(task *evo.TaskHandle) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		task.Doing("still working")
	}
}
