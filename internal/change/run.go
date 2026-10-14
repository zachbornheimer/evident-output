// This file owns Run, the change side of one Run: the workspace it resolves
// paths against and the state its operations keep per Task.

package change

import (
	"sync"

	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
	"github.com/zachbornheimer/evident-output/v2/internal/fs"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// workspaceFallback is the workspace of a Run whose working directory cannot
// be read: relative paths then resolve against the process's own.
const workspaceFallback = "."

// Run performs the changes of one Run. It is the owner every Define scope
// carries (see graph.Scope), so File, Exec, Patch and Effect find it from the
// context a Task's callback received.
//
// Its mutexes sit between the engine's and the graph's in the lock order
// (o.mu, then these, then graph.mu): none is held across a call into the
// Host or into the graph's gates, since a gate's unlock can deliver record
// listeners that take o.mu.
type Run struct {
	host  Host
	graph *graph.Graph

	barrier freshness.OutputBarrier

	outputsMu sync.Mutex
	// outputs records which Task first claimed each canonical File or Exec
	// output path in this Run (spec §11.4/§8.3): a second Task claiming the
	// same path is a producer conflict.
	outputs map[string]outputClaim
	// conflicts holds, for a Task whose claimed output another Task then
	// claimed, the error its own File call returns before it settles.
	conflicts map[record.TaskID]error

	workspaceMu  sync.Mutex
	workspaceDir string

	effects effectFlights
}

// NewRun is the change side of the Run whose Tasks g declares, reporting
// through host.
func NewRun(host Host, g *graph.Graph) *Run {
	return &Run{host: host, graph: g, barrier: freshness.NewOutputBarrier(g)}
}

// Workspace is the directory relative paths resolve against, captured once
// on first use so every relative path in this Run resolves against the same
// snapshot even if the process's working directory later changes (§8.1).
func (r *Run) Workspace() string {
	r.workspaceMu.Lock()
	defer r.workspaceMu.Unlock()
	if r.workspaceDir == "" {
		dir, err := fs.Getwd()
		if err != nil {
			dir = workspaceFallback
		}
		r.workspaceDir = dir
	}
	return r.workspaceDir
}

// PinWorkspace fixes the workspace to dir before any operation captures the
// process's own, so a test can point a Run at a temporary directory.
func (r *Run) PinWorkspace(dir string) {
	r.workspaceMu.Lock()
	defer r.workspaceMu.Unlock()
	r.workspaceDir = dir
}

// effectFlights counts, per Task, the Effect callbacks running and the
// times the Task disowned its own work while they ran.
type effectFlights struct {
	mu       sync.Mutex
	inFlight map[record.TaskID]int
	denials  map[record.TaskID]int
}

// enter notes one more Effect callback running for id and returns how many
// denials id has seen so far.
func (f *effectFlights) enter(id record.TaskID) (denialsAtEntry int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.inFlight == nil {
		f.inFlight, f.denials = make(map[record.TaskID]int), make(map[record.TaskID]int)
	}
	f.inFlight[id]++
	return f.denials[id]
}

// exit notes that an Effect callback for id returned, and reports whether
// id denied its own work since denialsAtEntry.
func (f *effectFlights) exit(id record.TaskID, denialsAtEntry int) (disowned bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	disowned = f.denials[id] != denialsAtEntry
	if f.inFlight[id]--; f.inFlight[id] <= 0 {
		delete(f.inFlight, id)
		delete(f.denials, id)
	}
	return disowned
}

func (f *effectFlights) active(id record.TaskID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.inFlight[id] > 0
}

func (f *effectFlights) deny(id record.TaskID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.inFlight[id] == 0 {
		return
	}
	f.denials[id]++
}

// EffectInFlight reports whether an Effect callback is running for id.
func (r *Run) EffectInFlight(id record.TaskID) bool { return r.effects.active(id) }

// DenyEffect records that id resolved itself, from inside its own callback,
// as anything but Done: every Effect in flight for id is disowned and
// records nothing. The caller has checked EffectInFlight.
func (r *Run) DenyEffect(id record.TaskID) { r.effects.deny(id) }
