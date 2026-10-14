package evo_test

import (
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
	"github.com/zachbornheimer/evident-output/v2/testkit"
)

// declaredInCallbackRemedy is the misuse remedy a declaration from inside a
// callback prints; a Confirm asked there must never earn it.
const declaredInCallbackRemedy = "a Task's Define callback only does work"

// confirmFromDefine runs one Task whose Define asks question, as CONTRACT
// §5's example does, and returns Refuse when the answer is no.
func confirmFromDefine(t *testing.T, cfg evo.Config) (contractRun, bool) {
	t.Helper()
	var answered bool
	run := executeContract(context.Background(), t, cfg, func(_ context.Context, r *evo.Run) error {
		r.Task("delete branches").Define(func(context.Context) error {
			answered = r.Confirm("delete 40 branches?", evo.Destructive(), evo.NonInteractive("--yes"))
			if !answered {
				return evo.Refuse("declined")
			}
			return nil
		})
		return nil
	})
	return run, answered
}

// TestConfirmInsideDefineIsNotMisuse pins CONTRACT §5: Confirm asked from a
// Define callback prompts and answers like one asked from the run body. It
// records no declaration misuse, and an unattended run still refuses with the
// hint that confirms non-interactively.
func TestConfirmInsideDefineIsNotMisuse(t *testing.T) {
	t.Run("answered yes", func(t *testing.T) {
		screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
		run, yes := confirmFromDefine(t, evo.Config{Name: "confirm", Facades: evo.Facades{Terminal: screen, Stdin: strings.NewReader("y\n")}})
		if !yes {
			t.Fatalf("Confirm answered y returned false:\n%s%s", run.Text(), screen.PersistedText())
		}
		if code := run.Result.ExitCode(); code != 0 {
			t.Errorf("exit code = %d, want 0:\n%s%s", code, run.Text(), screen.PersistedText())
		}
		assertNoDeclarationMisuse(t, run.Text()+screen.PersistedText())
	})
	t.Run("unattended", func(t *testing.T) {
		run, yes := confirmFromDefine(t, evo.Config{Name: "confirm", Output: evo.Plain})
		if yes {
			t.Fatal("Confirm with nobody to answer returned true")
		}
		if code := run.Result.ExitCode(); code != 1 {
			t.Errorf("exit code = %d, want 1 (Refused):\n%s", code, run.Text())
		}
		if !strings.Contains(run.Text(), "--yes") {
			t.Errorf("an unattended refusal does not point at --yes:\n%s", run.Text())
		}
		assertNoDeclarationMisuse(t, run.Text())
	})
}

func assertNoDeclarationMisuse(t *testing.T, text string) {
	t.Helper()
	if strings.Contains(text, declaredInCallbackRemedy) {
		t.Errorf("Confirm inside Define recorded declaration misuse:\n%s", text)
	}
}
