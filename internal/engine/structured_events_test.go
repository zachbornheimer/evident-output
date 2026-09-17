package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/wire"
)

// wireEventLine is one decoded "evo.event" JSONL row, for test assertions.
type wireEventLine struct {
	Object  string         `json:"object"`
	Seq     uint64         `json:"seq"`
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

// decodeWireEvents parses every JSONL line in body as a wireEventLine,
// failing the test on the first line that is not valid "evo.event" JSON.
func decodeWireEvents(t *testing.T, body string) []wireEventLine {
	t.Helper()
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return nil
	}
	lines := strings.Split(body, "\n")
	events := make([]wireEventLine, 0, len(lines))
	for i, line := range lines {
		var e wireEventLine
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("line %d is not valid JSON: %v\nline: %s", i, err, line)
		}
		if e.Object != wire.EventObject {
			t.Fatalf("line %d object = %q, want %q", i, e.Object, wire.EventObject)
		}
		events = append(events, e)
	}
	return events
}

// wireEventTypes returns just the Type sequence, for exact-order assertions.
func wireEventTypes(events []wireEventLine) []string {
	types := make([]string, len(events))
	for i, e := range events {
		types[i] = e.Type
	}
	return types
}

// indexOfType returns the first index of typ in events, or -1.
func indexOfType(events []wireEventLine, typ string) int {
	for i, e := range events {
		if e.Type == typ {
			return i
		}
	}
	return -1
}

func assertTypesEqual(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("event sequence length = %d, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event[%d] = %q, want %q\ngot:  %v\nwant: %v", i, got[i], want[i], got, want)
		}
	}
}

// TestWireEvents_SuccessOrdering proves the §38 family sequence for the
// simplest scripted run: one Task, Define succeeds, no Verify, no tracked
// operation — the exact deterministic backbone every richer scenario below
// still contains.
func TestWireEvents_SuccessOrdering(t *testing.T) {
	var stdout nopFlushWriter
	out := Init(Config{Isolated: true, Format: FormatJSONL, Stdout: &stdout})
	task := out.Task("build")
	task.Define(func(context.Context) error { return nil })
	_ = task.Wait()
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	events := decodeWireEvents(t, stdout.String())
	assertTypesEqual(t, wireEventTypes(events),
		wire.EventRunStarted,
		wire.EventTaskDeclared,
		wire.EventTaskEligible,
		wire.EventTaskStarted,
		wire.EventDefinitionStarted,
		wire.EventDefinitionFinished,
		wire.EventTaskFinished,
		wire.EventRunFinished,
	)
}

// TestWireEvents_AlreadySatisfiedSkipsDefinition proves spec §38's "whole
// Task skipped from current pre-definition Verify Evidence" distinction:
// a satisfied pre-Define Verify resolves the Task without ever entering
// Define, so no definition.started/definition.finished appear, and
// task.finished carries resolution=already_satisfied.
func TestWireEvents_AlreadySatisfiedSkipsDefinition(t *testing.T) {
	var stdout nopFlushWriter
	out := Init(Config{Isolated: true, Format: FormatJSONL, Stdout: &stdout})
	task := out.Task("build")
	task.Verify(func(context.Context) (bool, error) { return true, nil })
	task.Define(func(context.Context) error {
		t.Fatal("Define callback must not run when Verify is already satisfied")
		return nil
	})
	_ = task.Wait()
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	events := decodeWireEvents(t, stdout.String())
	types := wireEventTypes(events)
	if idx := indexOfType(events, wire.EventDefinitionStarted); idx != -1 {
		t.Fatalf("definition.started must not fire when Verify is already satisfied, found at %d: %v", idx, types)
	}
	evIdx := indexOfType(events, wire.EventEvidenceEvaluated)
	if evIdx == -1 {
		t.Fatalf("evidence.evaluated must fire, got: %v", types)
	}
	ev := events[evIdx]
	if ev.Payload["phase"] != "before_definition" || ev.Payload["satisfied"] != true {
		t.Fatalf("evidence.evaluated payload = %+v, want phase=before_definition satisfied=true", ev.Payload)
	}
	finIdx := indexOfType(events, wire.EventTaskFinished)
	if finIdx == -1 || events[finIdx].Payload["resolution"] != string(ResolutionAlreadySatisfied) {
		t.Fatalf("task.finished payload = %+v, want resolution=%s", events[finIdx].Payload, ResolutionAlreadySatisfied)
	}
	if evIdx >= finIdx {
		t.Fatalf("evidence.evaluated (%d) must precede task.finished (%d): %v", evIdx, finIdx, types)
	}
}

// TestWireEvents_FileOperationSkippedCurrent proves spec §38's "nested
// operation skipped as current"/"upstream revalidated with identical
// output" case: a second Run sharing one manifest Store hits the freshness
// fast path and emits operation.skipped_current instead of
// operation.started, with no live tracked_resource.observed (no inspection
// occurred).
func TestWireEvents_FileOperationSkippedCurrent(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(t.TempDir(), "managed.txt")
	spec := FileSpec{Path: path, Contents: []byte("desired")}

	first := Init(Config{Isolated: true, StateDir: state})
	if err := runFileTask(t, first, "file", spec); err != nil {
		t.Fatalf("first run: %v", err)
	}
	_ = first.Close()

	var stdout nopFlushWriter
	second := Init(Config{Isolated: true, StateDir: state, Format: FormatJSONL, Stdout: &stdout})
	if err := runFileTask(t, second, "file", spec); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if err := second.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	events := decodeWireEvents(t, stdout.String())
	types := wireEventTypes(events)
	skipIdx := indexOfType(events, wire.EventOperationSkippedCurrent)
	if skipIdx == -1 {
		t.Fatalf("operation.skipped_current must fire on the freshness fast path, got: %v", types)
	}
	if reason := events[skipIdx].Payload["reason"]; reason != freshnessReasonCurrent {
		t.Fatalf("operation.skipped_current reason = %v, want %q", reason, freshnessReasonCurrent)
	}
	if idx := indexOfType(events, wire.EventOperationStarted); idx != -1 {
		t.Fatalf("operation.started must not also fire for a skipped-current call, got: %v", types)
	}
	if idx := indexOfType(events, wire.EventTrackedResourceObserved); idx != -1 {
		t.Fatalf("tracked_resource.observed must not fire when no live inspection occurred, got: %v", types)
	}
	if indexOfType(events, wire.EventBasisFingerprinted) == -1 {
		t.Fatalf("basis.fingerprinted must still fire (computed ahead of the freshness check), got: %v", types)
	}
}

// TestWireEvents_FileOperationStartedAndFinished proves the reconciling
// (write-needed) path: operation.started names why the manifest considered
// this not current, tracked_resource.observed reports the live inspection,
// effect.committed records the write, and operation.finished closes it out
// changed=true.
func TestWireEvents_FileOperationStartedAndFinished(t *testing.T) {
	var stdout nopFlushWriter
	out := Init(Config{Isolated: true, StateDir: t.TempDir(), Format: FormatJSONL, Stdout: &stdout})
	path := filepath.Join(t.TempDir(), "managed.txt")
	spec := FileSpec{Path: path, Contents: []byte("desired")}
	if err := runFileTask(t, out, "file", spec); err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	events := decodeWireEvents(t, stdout.String())
	types := wireEventTypes(events)
	startIdx := indexOfType(events, wire.EventOperationStarted)
	if startIdx == -1 {
		t.Fatalf("operation.started must fire for a first-time File call, got: %v", types)
	}
	if reason := events[startIdx].Payload["reason"]; reason != freshnessReasonNoPriorRecord {
		t.Fatalf("operation.started reason = %v, want %q", reason, freshnessReasonNoPriorRecord)
	}
	trackedIdx := indexOfType(events, wire.EventTrackedResourceObserved)
	effectIdx := indexOfType(events, wire.EventEffectCommitted)
	finishIdx := indexOfType(events, wire.EventOperationFinished)
	if trackedIdx == -1 || effectIdx == -1 || finishIdx == -1 {
		t.Fatalf("expected tracked_resource.observed, effect.committed, operation.finished all present, got: %v", types)
	}
	if startIdx >= trackedIdx || trackedIdx >= effectIdx || effectIdx >= finishIdx {
		t.Fatalf("expected operation.started < tracked_resource.observed < effect.committed < operation.finished, got: %v", types)
	}
	if changed := events[finishIdx].Payload["changed"]; changed != true {
		t.Fatalf("operation.finished changed = %v, want true", changed)
	}
	if indexOfType(events, wire.EventManifestTaskCommitted) == -1 {
		t.Fatalf("manifest.task_committed must fire once the Task settles Done, got: %v", types)
	}
}

// TestWireEvents_ExecOperationStartedAndFinished proves Exec emits the same
// §38 operation family File does: basis.fingerprinted ahead of the
// freshness check, operation.started naming why the manifest considered
// this not current, tracked_resource.observed for the declared Output,
// operation.finished changed=true, and manifest.task_committed once the
// Task settles.
func TestWireEvents_ExecOperationStartedAndFinished(t *testing.T) {
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	outPath := filepath.Join(dir, "out.txt")
	runner := &scriptedRunner{exitCode: 0}

	var stdout nopFlushWriter
	out := Init(Config{
		Isolated: true, StateDir: t.TempDir(), ProcessRunner: runner,
		Format: FormatJSONL, Stdout: &stdout,
	})
	spec := ExecSpec{Executable: tool, Outputs: []string{"out.txt"}, Dir: dir}
	runner.onRun = func() {
		if err := os.WriteFile(outPath, []byte("built"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := runExecTask(t, out, "build", spec); err != nil {
		t.Fatalf("run: %v", err)
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	events := decodeWireEvents(t, stdout.String())
	types := wireEventTypes(events)
	basisIdx := indexOfType(events, wire.EventBasisFingerprinted)
	startIdx := indexOfType(events, wire.EventOperationStarted)
	trackedIdx := indexOfType(events, wire.EventTrackedResourceObserved)
	finishIdx := indexOfType(events, wire.EventOperationFinished)
	committedIdx := indexOfType(events, wire.EventManifestTaskCommitted)
	if basisIdx == -1 || startIdx == -1 || trackedIdx == -1 || finishIdx == -1 || committedIdx == -1 {
		t.Fatalf("expected basis.fingerprinted, operation.started, tracked_resource.observed, operation.finished, manifest.task_committed all present, got: %v", types)
	}
	if basisIdx >= startIdx || startIdx >= trackedIdx || trackedIdx >= finishIdx || finishIdx >= committedIdx {
		t.Fatalf("expected basis.fingerprinted < operation.started < tracked_resource.observed < operation.finished < manifest.task_committed, got: %v", types)
	}
	if reason := events[startIdx].Payload["reason"]; reason != freshnessReasonNoPriorRecord {
		t.Fatalf("operation.started reason = %v, want %q", reason, freshnessReasonNoPriorRecord)
	}
	if changed := events[finishIdx].Payload["changed"]; changed != true {
		t.Fatalf("operation.finished changed = %v, want true", changed)
	}
}

// TestWireEvents_ExecOperationSkippedCurrent proves the freshness fast path:
// a second Run sharing one manifest Store hits execOperationCurrent and
// emits operation.skipped_current instead of operation.started, with no
// live tracked_resource.observed (no re-inspection occurred).
func TestWireEvents_ExecOperationSkippedCurrent(t *testing.T) {
	state := t.TempDir()
	dir := t.TempDir()
	tool := execFixture(t, dir, "tool", "v1")
	outPath := filepath.Join(dir, "out.txt")
	spec := ExecSpec{Executable: tool, Outputs: []string{"out.txt"}, Dir: dir}

	first := Init(Config{Isolated: true, StateDir: state, ProcessRunner: &scriptedRunner{exitCode: 0}})
	if err := os.WriteFile(outPath, []byte("built"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runExecTask(t, first, "build", spec); err != nil {
		t.Fatalf("first run: %v", err)
	}
	_ = first.Close()

	var stdout nopFlushWriter
	second := Init(Config{
		Isolated: true, StateDir: state, ProcessRunner: &scriptedRunner{exitCode: 0},
		Format: FormatJSONL, Stdout: &stdout,
	})
	if err := runExecTask(t, second, "build", spec); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if err := second.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	events := decodeWireEvents(t, stdout.String())
	types := wireEventTypes(events)
	skipIdx := indexOfType(events, wire.EventOperationSkippedCurrent)
	if skipIdx == -1 {
		t.Fatalf("operation.skipped_current must fire on the freshness fast path, got: %v", types)
	}
	if reason := events[skipIdx].Payload["reason"]; reason != freshnessReasonCurrent {
		t.Fatalf("operation.skipped_current reason = %v, want %q", reason, freshnessReasonCurrent)
	}
	if idx := indexOfType(events, wire.EventOperationStarted); idx != -1 {
		t.Fatalf("operation.started must not also fire for a skipped-current call, got: %v", types)
	}
	if idx := indexOfType(events, wire.EventTrackedResourceObserved); idx != -1 {
		t.Fatalf("tracked_resource.observed must not fire when no live inspection occurred, got: %v", types)
	}
	if indexOfType(events, wire.EventBasisFingerprinted) == -1 {
		t.Fatalf("basis.fingerprinted must still fire (computed ahead of the freshness check), got: %v", types)
	}
}

// TestWireEvents_RunFinishedOnFailure and TestWireEvents_RunFinishedOnCancel
// prove run.finished fires (spec §38) on every terminal path, not only the
// clean-success one.

func TestWireEvents_RunFinishedOnFailure(t *testing.T) {
	var stdout nopFlushWriter
	out := Init(Config{Isolated: true, Format: FormatJSONL, Stdout: &stdout})
	out.Task("build").Fail("broken")
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	events := decodeWireEvents(t, stdout.String())
	idx := indexOfType(events, wire.EventRunFinished)
	if idx == -1 {
		t.Fatalf("run.finished must fire on a failed run, got: %v", wireEventTypes(events))
	}
	if events[idx].Payload["outcome"] != wire.OutcomeFailed {
		t.Fatalf("run.finished outcome = %v, want %q", events[idx].Payload["outcome"], wire.OutcomeFailed)
	}
	if idx != len(events)-1 {
		t.Fatalf("run.finished must be the last event, got index %d of %d", idx, len(events))
	}
}

func TestWireEvents_RunFinishedOnCancel(t *testing.T) {
	var stdout nopFlushWriter
	out := Init(Config{Isolated: true, Format: FormatJSONL, Stdout: &stdout})
	task := out.Task("build")
	out.Cancel("interrupted")
	_ = task
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	events := decodeWireEvents(t, stdout.String())
	idx := indexOfType(events, wire.EventRunFinished)
	if idx == -1 {
		t.Fatalf("run.finished must fire on a cancelled run, got: %v", wireEventTypes(events))
	}
	if events[idx].Payload["outcome"] != wire.OutcomeCancelled {
		t.Fatalf("run.finished outcome = %v, want %q", events[idx].Payload["outcome"], wire.OutcomeCancelled)
	}
}

// TestWireEvents_SeqStrictlyMonotonic_UnderConcurrentTasks proves seq is
// the strict, race-free ordering authority (spec §38) even when many Tasks
// resolve from concurrent scheduler goroutines — run with -race.
func TestWireEvents_SeqStrictlyMonotonic_UnderConcurrentTasks(t *testing.T) {
	const taskCount = 40
	var stdout nopFlushWriter
	out := Init(Config{Isolated: true, Format: FormatJSONL, Stdout: &stdout, MaxConcurrency: 8})

	var wg sync.WaitGroup
	for i := 0; i < taskCount; i++ {
		task := out.Task(fmt.Sprintf("task-%d", i))
		wg.Add(1)
		task.Define(func(context.Context) error {
			defer wg.Done()
			return nil
		})
	}
	wg.Wait()
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	events := decodeWireEvents(t, stdout.String())
	seen := make(map[uint64]bool, len(events))
	var lastSeq uint64
	for i, e := range events {
		if seen[e.Seq] {
			t.Fatalf("duplicate seq %d at event %d (%s)", e.Seq, i, e.Type)
		}
		seen[e.Seq] = true
		if e.Seq <= lastSeq {
			t.Fatalf("seq %d at event %d (%s) is not strictly increasing after %d", e.Seq, i, e.Type, lastSeq)
		}
		lastSeq = e.Seq
	}
	if len(events) < taskCount*4 {
		t.Fatalf("expected at least %d events for %d concurrent tasks, got %d", taskCount*4, taskCount, len(events))
	}
}

// failAfterNWriter succeeds its first n Write calls, then fails every call
// after — a stand-in for a stdout pipe that breaks mid-stream (spec §32.2).
type failAfterNWriter struct {
	buf   bytes.Buffer
	n     int
	calls int
}

func (w *failAfterNWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls > w.n {
		return 0, errors.New("failAfterNWriter: simulated write failure")
	}
	return w.buf.Write(p)
}

// TestWireEvents_WriteFailureMidStream_FailsRunButKeepsEarlierLines proves
// spec §32.2: "Earlier valid lines remain valid if a later write fails; the
// Run then fails" — the lines already written stay intact, and Finish
// reports a non-nil error escalating the run's exit code.
func TestWireEvents_WriteFailureMidStream_FailsRunButKeepsEarlierLines(t *testing.T) {
	w := &failAfterNWriter{n: 2}
	out := Init(Config{Isolated: true, Format: FormatJSONL, Stdout: w})
	task := out.Task("build")
	task.Define(func(context.Context) error { return nil })
	_ = task.Wait()

	finishErr := out.Finish()
	if finishErr == nil {
		t.Fatalf("Finish must report an error after a mid-stream wire-event write failure")
	}

	written := w.buf.String()
	lines := strings.Split(strings.TrimRight(written, "\n"), "\n")
	if len(lines) != w.n {
		t.Fatalf("expected exactly %d valid earlier lines preserved, got %d:\n%s", w.n, len(lines), written)
	}
	for i, line := range lines {
		var e wireEventLine
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("earlier line %d was corrupted by the later failure: %v\nline: %s", i, err, line)
		}
	}
}
