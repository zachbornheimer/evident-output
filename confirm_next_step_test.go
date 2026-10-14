package evo

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// confirmNextStepRule is the sentence CONTRACT §5 states for when a refused
// Confirm points at a next step.
const confirmNextStepRule = "the Next step appears only when nobody could answer"

// confirmRefusal asks one Confirm that names "--yes", with reply as the
// whole of stdin, and returns what a person reads on the finished screen
// and the refused row's Next actions.
func confirmRefusal(t *testing.T, reply string) (screenText string, actions int) {
	t.Helper()
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := initRun(Config{Name: "demo", Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: screen, Stdin: strings.NewReader(reply)}}, tuning{color: colorNever})
	if out.Confirm("delete 40 branches?", Destructive(), NonInteractive("--yes")) {
		t.Fatalf("Confirm with stdin %q = true, want a refusal", reply)
	}
	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	item := out.snapshot().Tasks[0]
	if item.State != refused || len(item.Problems) == 0 {
		t.Fatalf("stdin %q: want a Refused row with its Problem, got %+v", reply, item)
	}
	return screen.PersistedText() + screen.FinalText(), len(item.Problems[0].Actions)
}

// TestDeclinedConfirmPrintsNoNextStep pins both halves of CONTRACT §5's
// rule: a person who declined chose, so the row carries no Next step; a
// Confirm nobody could answer (stdin closed here, no terminal in the other
// test) carries the non-interactive flag, and CONTRACT.md states the rule.
func TestDeclinedConfirmPrintsNoNextStep(t *testing.T) {
	declined, declinedActions := confirmRefusal(t, "n\n")
	if declinedActions != 0 || strings.Contains(declined, "--yes") {
		t.Errorf("a declined Confirm must print no Next step (%d actions):\n%s", declinedActions, declined)
	}

	unanswerable, unanswerableActions := confirmRefusal(t, "")
	if unanswerableActions == 0 || !strings.Contains(unanswerable, "demo --yes") {
		t.Errorf("an unanswerable Confirm must print the non-interactive Next step (%d actions):\n%s", unanswerableActions, unanswerable)
	}

	contract, err := os.ReadFile("CONTRACT.md")
	if err != nil {
		t.Fatalf("read CONTRACT.md: %v", err)
	}
	if !strings.Contains(string(contract), confirmNextStepRule) {
		t.Errorf("CONTRACT.md must state the rule: %q", confirmNextStepRule)
	}
}
