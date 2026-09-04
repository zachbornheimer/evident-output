package evo

import (
	"bytes"
	"strings"
	"testing"
)

// TestFailf_NoTitle_UsesExecutableBasename is I2: the synthetic
// Output-level failure/cancel task, when no Task name and no explicit
// Config.Title identify it, uses the caller's actual executable basename
// (via the processArgv0 facade) instead of the generic literal "command".
func TestFailf_NoTitle_UsesExecutableBasename(t *testing.T) {
	restore := processArgv0
	processArgv0 = func() string { return "/usr/local/bin/clean-repo" }
	defer func() { processArgv0 = restore }()

	var buf bytes.Buffer
	out := newOutput("", To(&buf), NoColor(), Plain())
	out.Failf("boom")
	_ = out.Finish()

	if !strings.Contains(buf.String(), "clean-repo") {
		t.Fatalf("expected the executable basename \"clean-repo\", got:\n%s", buf.String())
	}
}
