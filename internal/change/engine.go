// This file owns the Host a Run's changes get in production: the one built
// over Engine, the little the root lends them. The record writes a change
// makes (its ledger rows, its sections and its verification details) and the
// manifest calls it makes go through here, so the root writes none of them.

package change

import (
	"context"
	"sync"

	"github.com/zachbornheimer/evident-output/internal/freshness"
	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// Engine is what the run around a Run's changes lends them. The root
// implements it; change never imports the root.
type Engine interface {
	Settings
	Activity
	Spawner
	// Lock and Unlock bracket one operation under the run's lock, which
	// comes before graph's and record's in the lock order.
	sync.Locker
	// LedgerTense resolves id to the tense its ledger rows take. It fails
	// when the run is closed or the Task already resolved, recording misuse
	// unless the Task is simply unknown. The caller holds the lock.
	LedgerTense(id record.TaskID) (record.LedgerTense, error)
	// CheckOpen fails, recording misuse, once the run is closing. The
	// caller holds the lock.
	CheckOpen() error
	// Narration hears what changes did. The caller holds the lock.
	Narration
	// RecordMisuse reports a program error that belongs to the run.
	RecordMisuse(err error)
	// NoteManifestWarning surfaces the manifest's cache-miss warning. It is
	// called once per run at most.
	NoteManifestWarning(warning error)
}

// Narration is how the reader learns what changes did: each step of a
// tracked operation, and each record write a change made.
type Narration interface {
	// Observe narrates one step of a tracked operation.
	Observe(Observation)
	// SectionOpened reports that task's ledger section in tense opened.
	SectionOpened(task record.TaskID, section string, tense record.LedgerTense)
	// EntryRecorded reports a ledger row task recorded.
	EntryRecorded(task record.TaskID, row LedgerRow)
	// VerificationRecorded reports the verification details task's record
	// now holds, as stored.
	VerificationRecorded(task record.TaskID, details []record.VerificationDetail)
}

// LedgerRow is one ledger row as it was recorded.
type LedgerRow struct {
	// Section is the id of the section the row went to.
	Section string
	Tense   record.LedgerTense
	// Verb is the entry's verb in the section's tense.
	Verb  string
	Entry record.LedgerEntry
	// Added is false when the section already held the row.
	Added bool
}

// HostParts are what NewHost builds a Host from.
type HostParts struct {
	Engine    Engine
	Record    *record.Run
	Graph     *graph.Graph
	Freshness *freshness.Run
}

// engineHost is the Host NewHost builds.
type engineHost struct {
	Engine
	record    *record.Run
	graph     *graph.Graph
	freshness *freshness.Run
}

// NewHost is the Host of a Run whose root lends parts.Engine.
func NewHost(parts HostParts) Host {
	return &engineHost{Engine: parts.Engine, record: parts.Record, graph: parts.Graph, freshness: parts.Freshness}
}

// taskLedger is one Task's ledger section, resolved once.
type taskLedger struct {
	host  *engineHost
	task  record.TaskID
	tense record.LedgerTense
}

// Applies reports whether the ledger records changes made, not planned.
func (l taskLedger) Applies() bool { return l.tense == record.TenseChanged }

// Record adds e to the Task's section, opening it on first use, with the
// verb in the section's tense. The imperative verb stays the section's
// intended verb, so a section that ends with no rows still reads "nothing to
// <verb> <subject>".
func (l taskLedger) Record(e record.LedgerEntry) {
	h := l.host
	verb := l.tense.Conjugate(e.Verb())
	h.Lock()
	defer h.Unlock()
	if h.CheckOpen() != nil {
		return
	}
	section := h.sectionLocked(l.task, l.tense)
	added := h.record.RecordEntry(string(l.task), l.tense, verb, e)
	h.EntryRecorded(l.task, LedgerRow{Section: section, Tense: l.tense, Verb: verb, Entry: e, Added: added})
}

// sectionLocked is the id of task's section in tense, opened on first use.
// The caller holds the lock.
func (h *engineHost) sectionLocked(task record.TaskID, tense record.LedgerTense) string {
	if section, ok := h.record.SectionID(string(task), tense); ok {
		return section
	}
	section := h.record.OpenSection(h.graph.Task(string(task)).LedgerOwner(), tense, h.graph.NextID(tense.String()))
	h.SectionOpened(task, section, tense)
	return section
}

// Ledger resolves id's ledger section once, before the change runs: a
// re-resolution afterwards could find the Task concurrently resolved and drop
// a real change as misuse.
func (h *engineHost) Ledger(id record.TaskID) (Ledger, error) {
	h.Lock()
	defer h.Unlock()
	tense, err := h.LedgerTense(id)
	if err != nil {
		return nil, err
	}
	return taskLedger{host: h, task: id, tense: tense}, nil
}

// AttachVerification adds details to id's record before it resolves, so the
// evidence is there by the time a terminal verb reads it. A Task that is
// unknown or already resolved keeps nothing.
func (h *engineHost) AttachVerification(id record.TaskID, details []record.VerificationDetail) {
	h.Lock()
	defer h.Unlock()
	t := h.graph.Task(string(id))
	if t == nil || record.IsTerminalTask(t.Rec.State()) {
		return
	}
	h.VerificationRecorded(id, t.Rec.AttachVerification(details))
}

// Observe narrates one step under the run's lock.
func (h *engineHost) Observe(ob Observation) {
	h.Lock()
	defer h.Unlock()
	h.Engine.Observe(ob)
}

// TaskName is the name id's row shows, or id itself when it is unknown.
func (h *engineHost) TaskName(id record.TaskID) string {
	if t := h.graph.Task(string(id)); t != nil {
		return t.Name
	}
	return string(id)
}

// OverturnTask fails id with err, settled or not (see graph.Graph.Overturn).
// It takes the run's lock itself.
func (h *engineHost) OverturnTask(id record.TaskID, err error) {
	if t := h.graph.Task(string(id)); t != nil {
		h.graph.Overturn(t, err)
	}
}

// Manifest opens the run's manifest on first use.
func (h *engineHost) Manifest(ctx context.Context) (*freshness.ManifestStore, error) {
	return h.freshness.Store(ctx)
}

// NoteManifestWarning surfaces a safe cache-miss warning once per run.
func (h *engineHost) NoteManifestWarning(warning error) {
	if warning == nil || !h.freshness.Session().FirstMissWarning() {
		return
	}
	h.Engine.NoteManifestWarning(warning)
}

// TaskManifest is id's stable manifest key and its next operation's ordinal.
func (h *engineHost) TaskManifest(id record.TaskID) (key string, ordinal int, ok bool) {
	return h.freshness.TaskManifest(id)
}

// AppendOperation records rec as id's next pending operation.
func (h *engineHost) AppendOperation(id record.TaskID, rec freshness.OperationRecord) {
	h.freshness.AppendOperation(id, rec)
}

// PlanOperation holds id's next operation's place without a record.
func (h *engineHost) PlanOperation(id record.TaskID, changes ...string) {
	h.freshness.PlanOperation(id, changes...)
}

// Plan is how the Run judges an operation's freshness.
func (h *engineHost) Plan() freshness.Plan { return h.freshness.Plan() }
