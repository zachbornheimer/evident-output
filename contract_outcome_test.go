package evo_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// callbackPatience bounds how long a callback waits for a cancellation the
// test triggers, so a cancellation that never arrives fails instead of hangs.
const callbackPatience = 5 * time.Second

// outcomeScenario is one run whose Conclusion must be want.
type outcomeScenario struct {
	want evo.Outcome
	run  func(t *testing.T, mode evo.Output) contractRun
}

// singleTask runs one Task "a" whose Verify and Define are given.
func singleTask(verify func(context.Context) (bool, error), define func(context.Context) error) func(*testing.T, evo.Output) contractRun {
	return func(t *testing.T, mode evo.Output) contractRun {
		return executeContract(context.Background(), t, evo.Config{Name: "outcome", Output: mode},
			func(_ context.Context, r *evo.Run) error {
				task := r.Task("a")
				if verify != nil {
					task.Verify(verify)
				}
				task.Define(define)
				return nil
			})
	}
}

// cancelledWhileRunning cancels the run's context once Task "a" is running.
func cancelledWhileRunning(t *testing.T, mode evo.Output) contractRun {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	go func() {
		<-started
		cancel()
	}()
	return executeContract(ctx, t, evo.Config{Name: "outcome", Output: mode},
		func(_ context.Context, r *evo.Run) error {
			r.Task("a").Define(func(taskCtx context.Context) error {
				close(started)
				select {
				case <-taskCtx.Done():
					return taskCtx.Err()
				case <-time.After(callbackPatience):
					return errors.New("the run's cancellation never reached the callback")
				}
			})
			return nil
		})
}

func outcomeScenarios() []outcomeScenario {
	succeed := func(context.Context) error { return nil }
	already := func(context.Context) (bool, error) { return true, nil }
	return []outcomeScenario{
		{evo.Succeeded, singleTask(nil, succeed)},
		{evo.Satisfied, singleTask(already, succeed)},
		{evo.Excluded, singleTask(nil, func(context.Context) error { return evo.Exclude("protected") })},
		{evo.Refused, singleTask(nil, func(context.Context) error { return evo.Refuse("no remote") })},
		{evo.Outcome(evo.Failed), singleTask(nil, func(context.Context) error { return errors.New("boom") })},
		{evo.Outcome(evo.Cancelled), cancelledWhileRunning},
	}
}

// allOutcomes is the closed set CONTRACT.md §4 lists.
func allOutcomes() []evo.Outcome {
	return []evo.Outcome{
		evo.Succeeded, evo.Satisfied, evo.Excluded, evo.Refused,
		evo.Outcome(evo.Failed), evo.Outcome(evo.Cancelled),
	}
}

// retiredVerdicts are the 1.x spellings of a verdict that CONTRACT.md §4
// replaces with Outcome. None may appear as a value on the wire.
var retiredVerdicts = map[string]bool{"ok": true, "done": true, "already_satisfied": true}

// TestConclusionOutcomeCoversAllSix pins CONTRACT.md §4: Outcome is the only
// verdict type, and each of its six values reaches Conclusion.Outcome and
// appears unchanged in Human, Plain, evo.run and evo.event.
func TestConclusionOutcomeCoversAllSix(t *testing.T) {
	for _, sc := range outcomeScenarios() {
		for _, mode := range contractOutputs {
			t.Run(fmt.Sprintf("%s/%s", sc.want, outputName[mode]), func(t *testing.T) {
				run := sc.run(t, mode)
				if got := run.Result.Conclusion.Outcome; got != sc.want {
					t.Errorf("Conclusion.Outcome = %q, want %q", got, sc.want)
				}
				switch mode {
				case evo.JSON:
					assertRunDocumentOutcome(t, runDocument(t, run.Stdout), sc.want)
				case evo.JSONL:
					assertEventOutcome(t, runEvents(t, run.Stdout), sc.want)
				default:
					assertTextNamesOutcome(t, run.Text(), sc.want)
				}
			})
		}
	}
}

// assertRunDocumentOutcome checks the run and its one Task carry want.
func assertRunDocumentOutcome(t *testing.T, doc map[string]any, want evo.Outcome) {
	t.Helper()
	if got := doc["outcome"]; got != string(want) {
		t.Errorf("evo.run outcome = %v, want %q", got, want)
	}
	for _, task := range documentTasks(t, doc) {
		if got := task["outcome"]; got != string(want) {
			t.Errorf("evo.run task %v outcome = %v, want %q", task["name"], got, want)
		}
	}
	for _, path := range retiredVerdictPaths("evo.run", doc) {
		t.Errorf("evo.run carries a retired verdict spelling at %s", path)
	}
}

// assertEventOutcome checks the task.finished and run.finished payloads.
func assertEventOutcome(t *testing.T, events []map[string]any, want evo.Outcome) {
	t.Helper()
	for _, typ := range []string{"task.finished", "run.finished"} {
		finished := eventsOfType(events, typ)
		if len(finished) == 0 {
			t.Errorf("no %s event", typ)
		}
		for _, event := range finished {
			if got := payload(event)["outcome"]; got != string(want) {
				t.Errorf("%s payload outcome = %v, want %q", typ, got, want)
			}
		}
	}
	for i, event := range events {
		for _, path := range retiredVerdictPaths(fmt.Sprintf("event[%d] %v", i, event["type"]), event) {
			t.Errorf("evo.event carries a retired verdict spelling at %s", path)
		}
	}
}

// assertTextNamesOutcome checks Human or Plain text names the Outcome in its
// own word. Succeeded is the one Outcome a ✓ row states alone, so its text
// must name none of the other five instead.
func assertTextNamesOutcome(t *testing.T, text string, want evo.Outcome) {
	t.Helper()
	if want != evo.Succeeded {
		if !outcomeWord(want).MatchString(text) {
			t.Errorf("text does not name the outcome %q:\n%s", want, text)
		}
		return
	}
	for _, other := range allOutcomes() {
		if other != evo.Succeeded && outcomeWord(other).MatchString(text) {
			t.Errorf("a Succeeded run's text names the outcome %q:\n%s", other, text)
		}
	}
}

func outcomeWord(o evo.Outcome) *regexp.Regexp {
	return regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(string(o)) + `\b`)
}

// retiredVerdictPaths lists where v holds a retired verdict string.
func retiredVerdictPaths(at string, v any) []string {
	switch x := v.(type) {
	case string:
		if retiredVerdicts[x] {
			return []string{fmt.Sprintf("%s = %q", at, x)}
		}
	case map[string]any:
		var out []string
		for k, child := range x {
			out = append(out, retiredVerdictPaths(at+"."+k, child)...)
		}
		return out
	case []any:
		var out []string
		for i, child := range x {
			out = append(out, retiredVerdictPaths(fmt.Sprintf("%s[%d]", at, i), child)...)
		}
		return out
	}
	return nil
}
