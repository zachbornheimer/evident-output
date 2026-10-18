package evo_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// failedCompileBesideGhost fails a Task and leaves another undefined: an earlier failure decides the
// Explanation, and the refused declaration still has to be recorded.
func failedCompileBesideGhost(t *testing.T, mode evo.Output) contractRun {
	return executeContract(context.Background(), t, evo.Config{Name: "dag", Output: mode},
		func(_ context.Context, r *evo.Run) error {
			r.Task("compile").Define(func(context.Context) error { return evo.Fail("compile failed") })
			r.Task("ghost")
			return nil
		})
}

// TestRefusedDeclarationHasOwnRecordInJSONAndJSONL pins CONTRACT section 7: a refusal that sits beside an
// earlier failure is a record of its own, a misuse entry in evo.run and a run.misuse event in JSONL, and
// both validate against the published schemas.
func TestRefusedDeclarationHasOwnRecordInJSONAndJSONL(t *testing.T) {
	doc := failedCompileBesideGhost(t, evo.JSON).Stdout
	validateAgainstPublished(t, publishedRunSchema, []byte(doc))
	var run struct {
		Data struct {
			Misuse []struct {
				Message string `json:"message"`
				Subject string `json:"subject"`
				Remedy  string `json:"remedy"`
			} `json:"misuse"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(doc), &run); err != nil {
		t.Fatal(err)
	}
	if len(run.Data.Misuse) != 1 || !strings.Contains(run.Data.Misuse[0].Message, `"ghost"`) ||
		run.Data.Misuse[0].Subject != "ghost" || run.Data.Misuse[0].Remedy == "" {
		t.Errorf("evo.run data.misuse = %+v, want one record naming ghost with a remedy:\n%s", run.Data.Misuse, doc)
	}

	var seen []string
	for line := range strings.SplitSeq(strings.TrimSpace(failedCompileBesideGhost(t, evo.JSONL).Stdout), "\n") {
		validateAgainstPublished(t, publishedEventSchema, []byte(line))
		var event struct {
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		seen = append(seen, event.Type)
		if event.Type == "run.misuse" && !strings.Contains(line, `"ghost"`) {
			t.Errorf("run.misuse does not name ghost: %s", line)
		}
	}
	if n := countOf(seen, "run.misuse"); n != 1 {
		t.Errorf("JSONL has %d run.misuse events, want 1: %v", n, seen)
	}
}

func countOf(all []string, want string) int {
	n := 0
	for _, s := range all {
		if s == want {
			n++
		}
	}
	return n
}
