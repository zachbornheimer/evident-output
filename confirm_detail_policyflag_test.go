package evo

import (
	"bytes"
	"strings"
	"testing"
)

// TestConfirm_Detail_RendersContextLines is I5: Detail("...")
// attaches context lines rendered under the "?  <question>  [y/N]" prompt.
// Uses the interactive ask-and-wait path (not the non-interactive policy
// block) since that's the only path that writes the prompt at all.
func TestConfirm_Detail_RendersContextLines(t *testing.T) {
	var buf bytes.Buffer
	restore := markWriterAsCharDevice(&buf)
	t.Cleanup(restore)
	out := initRun(Config{stdout: &buf, stderr: &buf, stdin: strings.NewReader("y\n"), color: colorNever})

	out.Confirm("delete origin/production-hotfix?",
		Detail("host: prod-db-1"), Detail("affects: 3 downstream services"))

	rendered := buf.String()
	if !strings.Contains(rendered, "host: prod-db-1") || !strings.Contains(rendered, "affects: 3 downstream services") {
		t.Fatalf("expected Detail context lines, got:\n%s", rendered)
	}
}

// TestConfirm_PolicyFlag_FillsExecutableFromTitle is I5: PolicyFlag
// fills the Next hint's executable from the same identity source Init/I2
// uses (Config.Title), instead of the caller hand-composing
// PolicyHint(os.Args[0], flag).
func TestConfirm_PolicyFlag_FillsExecutableFromTitle(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, title: "clean-repo", color: colorNever, plain: true})

	out.Confirm("delete 8 stale local branches?", policyFlag("--apply"))

	item := out.snapshot().Tasks[0]
	actions := policyBlockActions(t, item)
	if len(actions) == 0 || actions[0].Command == nil {
		t.Fatalf("actions = %+v, want a Command action", actions)
	}
	cmd := actions[0].Command
	if cmd.Executable != "clean-repo" || len(cmd.Args) != 1 || cmd.Args[0] != "--apply" {
		t.Fatalf("command = %+v, want executable %q with arg %q", cmd, "clean-repo", "--apply")
	}
}
