package retired

import "testing"

func TestUnexplainedInAllowsARemovalNote(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"use task.Record(verb, n, object) when the work already happened", 1},
		{"Record was removed in 1.1; task.Record(...) becomes evo.Effect", 0},
		{"range Group.Each so every iteration is visible", 1},
		{"Group.Each was removed in 1.0", 0},
		{"Group.Each was removed in 1.1", 1},
		{"Doing/Progress/Done already activate the task", 1},
		{"select { case <-ctx.Done(): }", 0},
		{"use Failf when formatting the summary", 1},
		{"Failf was removed in 1.1; Fail is a statement", 0},
		{"use Blockf for formatted refusals", 1},
		{"Blockf was removed in 1.1", 0},
		{"evo.KeepLastLines(100)", 1},
		{"KeepLastLines was removed in 1.1; use MaxCaptureBytes", 0},
	}
	for _, c := range cases {
		if got := len(UnexplainedIn(c.text)); got != c.want {
			t.Errorf("UnexplainedIn(%q) = %d hits, want %d", c.text, got, c.want)
		}
	}
}

func TestTaughtWarnAndCaptureEvidenceSpellings(t *testing.T) {
	hits := []string{
		"func (i *Item) Warn(summary string, options ...ProblemOption) *Item",
		"func (t *Task) Warn(summary string, options ...ProblemOption) *Task",
		"## Warn / Block / Fail",
		"| **Warn**                  | Proceed, but notice this                              |",
		`out.Task("credentials")  // condition — resolved directly (Done/Warn/Block/Fail/Skip)`,
		"type Evidence struct {",
		"## Evidence",
		"## Evidence ownership",
		"`Tasks`, `Group`, `Failure`, `Evidence`, `Config`, the functional-option",
		"    Evidence  []Evidence",
		"task.Evidence()",
		"Evidence belongs to the **entity** (a `Task`, whether it ran or was resolved as a",
	}
	for _, text := range hits {
		if len(TaughtIn(text)) == 0 {
			t.Errorf("TaughtIn(%q) missed a capture-meaning Warn/Evidence listing", text)
		}
	}
	misses := []string{
		"logger.Warn(msg)",
		"slog.Logger.Warn",
		"users should warn before deleting",
		"type EvidencePhase struct {",
		"## EvidencePhase",
		"Evidence TaskEvidence",
		"`Evidence` (`TaskEvidence`",
		"EvidenceTail string",
		"retains a bounded ring for Fail evidence",
		"func (i *Item) WarnedBy(problems ...Problem) *Item",
	}
	for _, text := range misses {
		for _, h := range TaughtIn(text) {
			if h.Symbol.Contract == "TaskHandle.Warn(" || h.Symbol.Contract == "Evidence" {
				t.Errorf("TaughtIn(%q) matched %q (%s); satisfaction-meaning and English must stay", text, h.Match, h.Symbol.Contract)
			}
		}
	}
}

func TestTaughtFailfBlockfKeepLastLines(t *testing.T) {
	hits := []string{
		"Failf",
		"Blockf",
		"KeepLastLines",
		"task.Failf(...)",
		"`Failf`",
		"`Blockf`",
		"evo.KeepLastLines(100)",
		"Failf/Blockf",
	}
	for _, text := range hits {
		if len(TaughtIn(text)) == 0 {
			t.Errorf("TaughtIn(%q) missed Failf/Blockf/KeepLastLines", text)
		}
	}
}

func TestEverySymbolHasAReleaseAndReplacement(t *testing.T) {
	for _, s := range Symbols() {
		if s.Replacement == "" || s.RemovedIn == "" {
			t.Errorf("%s: Replacement and RemovedIn are required", s.Contract)
		}
	}
}
