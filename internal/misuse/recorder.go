package misuse

import "sync"

// Recorder is the first-misuse store for one run. Any goroutine may record,
// including graph workers that hold no engine lock.
type Recorder struct {
	mu              sync.Mutex
	first           error
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

// Note records err as the run's misuse when it is the first, with the
// entity and rejected summary that name it.
func (r *Recorder) Note(err error, subject, rejectedSummary string) {
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
