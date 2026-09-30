package engine

import (
	"fmt"
	"reflect"
	"time"

	"github.com/zachbornheimer/evident-output/internal/render/live"
)

// auditedCollectionLimit keeps the audit's full walk to collections small
// enough that it does not distort the scale tests' timing.
const auditedCollectionLimit = 400

// init makes every live frame a test builds prove the childIndex current:
// the frame must equal the one a walk of every child builds.
func init() {
	liveIndexAudit = func(g *tasksState, rows int, now time.Time, got TasksSnapshot) {
		if len(g.tasks) > auditedCollectionLimit {
			return
		}
		children := live.NewLiveChildren(g.name, rows)
		for _, t := range g.tasks {
			if view := t.view(); children.Admit(&view) {
				children.Keep(t.snapshot())
			}
		}
		want := children.Collection(liveCollections(g.children, rows, now).Into(g.header()))
		if !reflect.DeepEqual(got, want) {
			panic(fmt.Sprintf("childIndex frame of %q differs from a walk of its children:\n got %+v\nwant %+v", g.name, got, want))
		}
	}
}
