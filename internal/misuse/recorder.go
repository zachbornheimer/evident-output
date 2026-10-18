package misuse

import (
	"errors"
	"sync"
)

// Recorder is the misuse store for one run. Any goroutine may record,
// including graph workers that hold no engine lock. It keeps every distinct
// misuse in record order; the first also names the corrective hint.
type Recorder struct {
	mu              sync.Mutex
	first           error
	all             []error
	seen            map[string]struct{}
	subject         string
	rejectedSummary string
}

// New is an empty Recorder.
func New() *Recorder {
	return &Recorder{}
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

// Note records err as one of the run's misuses, with the entity and rejected
// summary that name it when it is the first. A misuse whose text repeats an
// earlier one (the same Task resolved twice, many Tasks left unresolved) is
// kept once, so the joined error stays as long as the distinct problems.
func (r *Recorder) Note(err error, subject, rejectedSummary string) {
	if err == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.first == nil {
		r.first, r.subject, r.rejectedSummary = err, subject, rejectedSummary
	}
	text := err.Error()
	if _, repeat := r.seen[text]; repeat {
		return
	}
	if r.seen == nil {
		r.seen = map[string]struct{}{}
	}
	r.seen[text] = struct{}{}
	r.all = append(r.all, err)
}

// All is every distinct misuse recorded, joined in record order, nil when
// there is none.
func (r *Recorder) All() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return errors.Join(r.all...)
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
