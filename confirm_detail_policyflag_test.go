package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestConfirm_Detail_RendersContextLines is I5: evo.ConfirmDetail("...")
// attaches context lines rendered under the "?  <question>  [y/N]" prompt.
// Uses the interactive ask-and-wait path (not the non-interactive policy
// block) since that's the only path that writes the prompt at all.
func TestConfirm_Detail_RendersContextLines(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.To(&buf), evo.NoColor(), evo.Stdin(strings.NewReader("y\n")),
	}})

	out.Confirm("delete origin/production-hotfix?",
		evo.ConfirmDetail("host: prod-db-1", "affects: 3 downstream services"))

	rendered := buf.String()
	if !strings.Contains(rendered, "host: prod-db-1") || !strings.Contains(rendered, "affects: 3 downstream services") {
		t.Fatalf("expected ConfirmDetail context lines, got:\n%s", rendered)
	}
}

// TestConfirm_PolicyFlag_FillsExecutableFromTitle is I5: evo.PolicyFlag
// fills the Next hint's executable from the same identity source Init/I2
// uses (Config.Title), instead of the caller hand-composing
// PolicyHint(os.Args[0], flag).
func TestConfirm_PolicyFlag_FillsExecutableFromTitle(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.To(&buf), evo.NoColor(), evo.Plain(), evo.Title("clean-repo"),
	}})

	out.Confirm("delete 8 stale local branches?", evo.PolicyFlag("--apply"))

	item := out.Snapshot().Tasks[0]
	if len(item.Actions) == 0 || item.Actions[0].Command == nil {
		t.Fatalf("actions = %+v, want a Command action", item.Actions)
	}
	cmd := item.Actions[0].Command
	if cmd.Executable != "clean-repo" || len(cmd.Args) != 1 || cmd.Args[0] != "--apply" {
		t.Fatalf("command = %+v, want executable %q with arg %q", cmd, "clean-repo", "--apply")
	}
}
