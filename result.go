package evo

import (
	"fmt"
	"io"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// Result is what Execute returns: the run's Conclusion and Err, what the body returned joined with the
// first misuse the run recorded.
type Result struct {
	Conclusion Conclusion
	Err        error
}

func (r Result) ExitCode() int { return r.Conclusion.exitCode }

type Conclusion struct {
	Outcome     Outcome
	Changed     bool
	DryRun      bool
	Explanation string
	subject     string
	summary     string
	partial     bool
	warnings    []record.Problem
	cancelled   bool
	tasks       []taskSnapshot
	collections []tasksSnapshot
	changes     []changesSnapshot
	plans       []planSnapshot
	actions     []Action
	exitCode    int
	runID       string
	startedAt   time.Time
	finishedAt  time.Time
	facts       []factRecord
}

func asEvoConclusion(c record.Conclusion) Conclusion {
	return Conclusion{
		Outcome: Outcome(c.Outcome), Changed: c.Changed, DryRun: c.DryRun,
		Explanation: c.Explanation, subject: c.Subject, summary: c.Summary,
		partial: c.Partial, warnings: c.Warnings, cancelled: c.Cancelled,
		tasks: c.Tasks, collections: c.Collections, changes: c.Changes,
		plans: c.Plans, actions: asEvoActions(c.Actions), exitCode: c.ExitCode,
		runID: c.RunID, startedAt: c.StartedAt, finishedAt: c.FinishedAt,
		facts: c.Facts,
	}
}

func asRecordConclusion(c Conclusion) record.Conclusion {
	return record.Conclusion{
		Outcome: record.Outcome(c.Outcome), Subject: c.subject, Summary: c.summary, Changed: c.Changed, Partial: c.partial,
		Warnings: c.warnings, Cancelled: c.cancelled, Explanation: c.Explanation,
		Tasks: c.tasks, Collections: c.collections, Changes: c.changes,
		Plans: c.plans, Actions: asRecordActions(c.actions), ExitCode: c.exitCode,
		RunID: c.runID, StartedAt: c.startedAt, FinishedAt: c.finishedAt,
		DryRun: c.DryRun, Facts: c.facts,
	}
}

// ResultWriter returns the stdout data payload; presentation never writes to it. Human and Plain runs
// reserve stdout for it and pass the bytes through unchanged. A JSON run embeds what it receives as the
// evo.run document's result, and a JSONL run streams each write as a result.written event, so stdout stays
// machine-readable; both carry bytes that are not valid UTF-8 as base64. Writing after the run finished is
// misuse in every mode and returns an error. A run with no payload stream returns io.Discard.
func (r *Run) ResultWriter() io.Writer {
	if r == nil {
		return io.Discard
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cfg.result != nil || r.cfg.wireFormat == JSON || r.cfg.wireFormat == JSONL {
		return resultWriter{r}
	}
	return io.Discard
}

// resultWriter is the payload stream of a run: it folds a write into the machine document or event stream
// that owns stdout, routes it through the live region when stdout shares the region's terminal, and
// otherwise passes it to stdout as it is.
type resultWriter struct{ o *Run }

func (w resultWriter) Write(b []byte) (int, error) {
	sink, err := w.o.routeResult(b)
	if err != nil {
		return 0, err
	}
	if sink == nil {
		return len(b), nil
	}
	return sink.Write(b)
}

// routeResult delivers b when the run itself owns the destination, and otherwise returns the raw payload
// stream for the caller to write after the run lock is released, so a slow reader never stalls the run.
func (r *Run) routeResult(b []byte) (io.Writer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ensureOpen(); err != nil {
		r.recordMisuse(err)
		return nil, fmt.Errorf("write result: %w", err)
	}
	switch {
	case r.cfg.wireFormat == JSONL:
		r.emitWireEventLocked(wire.EventResultWritten, "", wire.NewResultWrittenPayload(b).EventPayload())
	case r.cfg.wireFormat == JSON:
		r.resultText.Write(b)
	case r.cfg.resultSharesTerminal && r.show.Stream.WritePayload(string(b)):
	default:
		return r.cfg.result, nil
	}
	return nil, nil
}
