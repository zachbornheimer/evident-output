package misuse

import "sync"

// Recorder is the first-misuse store for one run. Graph workers call it
// without the engine's mutex; a strict recorder panics after recording,
// so those calls must never run under graph.mu.
type Recorder struct {
	mu              sync.Mutex
	first           error
	subject         string
	rejectedSummary string
	strict          bool
}

// New is an empty Recorder. strict panics after the first record, once
// the store has been written, so Finish still sees the misuse.
func New(strict bool) *Recorder {
	return &Recorder{strict: strict}
}

// RecordMisuseFor reports err, naming the entity it happened on. It is the
// method graph.MisuseSink requires.
func (r *Recorder) RecordMisuseFor(subject string, err error) {
	r.Note(err, subject, "")
}

// Record is Note with no entity name.
func (r *Recorder) Record(err error) {
	r.Note(err, "", "")
}

// Note records err as the run's misuse when it is the first, with the
// entity and rejected summary that name it, and panics with it in strict
// mode. Any goroutine may call it.
func (r *Recorder) Note(err error, subject, rejectedSummary string) {
	if err == nil {
		return
	}
	r.Keep(err, subject, rejectedSummary)
	if r.strict {
		panic(err)
	}
}

// Keep records err as the run's misuse when it is the first, with the
// entity and rejected summary that name it, in every mode and without a
// strict panic: for a goroutine no caller's code is beneath to take one.
func (r *Recorder) Keep(err error, subject, rejectedSummary string) {
	if err == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.first == nil {
		r.first, r.subject, r.rejectedSummary = err, subject, rejectedSummary
	}
}

// First is the first misuse recorded, nil when there is none.
func (r *Recorder) First() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.first
}

// Hint is the one required corrective sentence for the first recorded
// misuse, empty when there is none.
func (r *Recorder) Hint() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.first == nil {
		return ""
	}
	return hintFor(r.first, r.subject, r.rejectedSummary)
}
