package tests22_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/wire"
)

const (
	probeCode    = "E_PROBE"
	probeSummary = "probe failed"
)

type projected struct {
	stdout     string
	conclusion evo.Conclusion
	state      evo.EntityState
}

// failedRun declares one succeeding and one failing task, where the failure
// carries probeCode, and returns what one projection wrote.
func failedRun(t *testing.T, format evo.Format, wording string) projected {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Color: evo.ColorNever, Format: format,
		Stdout: &buf, Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("fine").Define(func(context.Context) error { return nil })
	probe := out.Task("probe")
	probe.Define(func(context.Context) error {
		probe.Problem(wording, evo.Code(probeCode))
		return nil
	})
	_ = out.Finish()
	return projected{buf.String(), out.Conclusion(), probe.Snapshot().State}
}

func TestC22_020_TTYPlainJSONAndJSONLAgreeOnTheSameRun(t *testing.T) {
	human := failedRun(t, evo.FormatHuman, probeSummary)
	if !strings.Contains(human.stdout, "✗ probe") || !strings.Contains(human.stdout, probeSummary) {
		t.Fatalf("human projection lost the failed task or its problem:\n%s", human.stdout)
	}

	asJSON := failedRun(t, evo.FormatJSON, probeSummary)
	var doc wire.RunDocument
	if err := json.Unmarshal([]byte(asJSON.stdout), &doc); err != nil {
		t.Fatalf("decode run document: %v\n%s", err, asJSON.stdout)
	}
	if doc.ExitCode != human.conclusion.ExitCode {
		t.Fatalf("JSON exit_code = %d, human conclusion exit = %d", doc.ExitCode, human.conclusion.ExitCode)
	}
	if problems := taskProblems(doc); len(problems) != 1 || problems[0].Code != probeCode || problems[0].Message != probeSummary {
		t.Fatalf("JSON task problems = %+v, want the one %s problem", problems, probeCode)
	}
	if !taskStateMatches(doc, "probe", human.state) {
		t.Fatalf("JSON task states %+v disagree with the human state %s", doc.Data.Tasks, human.state)
	}

	asJSONL := failedRun(t, evo.FormatJSONL, probeSummary)
	requireEventStream(t, asJSONL.stdout)
	if asJSONL.conclusion.ExitCode != human.conclusion.ExitCode {
		t.Fatalf("JSONL conclusion exit = %d, human = %d", asJSONL.conclusion.ExitCode, human.conclusion.ExitCode)
	}
}

func taskProblems(doc wire.RunDocument) []wire.ProblemDoc {
	var problems []wire.ProblemDoc
	for _, task := range doc.Data.Tasks {
		problems = append(problems, task.Problems...)
	}
	return problems
}

func taskStateMatches(doc wire.RunDocument, name string, want evo.EntityState) bool {
	for _, task := range doc.Data.Tasks {
		if task.Name == name {
			return strings.EqualFold(task.State, string(want))
		}
	}
	return false
}

// requireEventStream checks every line is an evo.event, the stream ends the
// run, and the failed task's problem code and text reach the stream.
func requireEventStream(t *testing.T, stream string) {
	t.Helper()
	var types []string
	for line := range strings.SplitSeq(strings.TrimSpace(stream), "\n") {
		var event wire.EventDocument
		if err := json.Unmarshal([]byte(line), &event); err != nil || event.Object != wire.EventObject {
			t.Fatalf("not an evo.event line (%v): %s", err, line)
		}
		types = append(types, event.Type)
	}
	if len(types) == 0 || types[len(types)-1] != wire.EventRunFinished {
		t.Fatalf("event stream types %v do not end with %s", types, wire.EventRunFinished)
	}
	for _, want := range []string{probeCode, probeSummary} {
		if !strings.Contains(stream, want) {
			t.Fatalf("event stream does not carry %q:\n%s", want, stream)
		}
	}
}

func problemCodes(t *testing.T, stdout string) []string {
	t.Helper()
	var doc wire.RunDocument
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode run document: %v\n%s", err, stdout)
	}
	var codes []string
	for _, problem := range taskProblems(doc) {
		codes = append(codes, problem.Code)
	}
	return codes
}

func TestC22_021_ProblemCodesStayStableAcrossRunsAndWording(t *testing.T) {
	first := failedRun(t, evo.FormatJSON, probeSummary)
	again := failedRun(t, evo.FormatJSON, probeSummary)
	reworded := failedRun(t, evo.FormatJSON, "the probe did not pass")
	for name, run := range map[string]projected{"rerun": again, "reworded": reworded} {
		got := problemCodes(t, run.stdout)
		if len(got) != 1 || got[0] != probeCode {
			t.Errorf("%s problem codes = %v, want [%s]", name, got, probeCode)
		}
	}
	if got := problemCodes(t, first.stdout); len(got) != 1 || got[0] != probeCode {
		t.Fatalf("first run problem codes = %v, want [%s]", got, probeCode)
	}
}
