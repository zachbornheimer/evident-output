package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
	"github.com/zachbornheimer/evident-output/internal/agent/rules"
)

// TestMigration1_1_DirtyFiresAndRewriteIsClean proves each 1.1 removal:
// dirty source fires API-032 with an exact rewrite; applying it and
// re-reviewing yields no findings (recheck_required=false); rules.Explain
// resolves the cited id; a v1.0.0 pin does not get the 1.1 rewrite.
func TestMigration1_1_DirtyFiresAndRewriteIsClean(t *testing.T) {
	cases := []struct {
		name, dirty, clean, wantInSuggestion string
	}{
		{
			name: "Failf",
			dirty: `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
	task.Failf("boom")
}
`,
			clean: `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
	task.Fail("boom")
}
`,
			wantInSuggestion: `task.Fail("boom")`,
		},
		{
			name: "Blockf",
			dirty: `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
	task.Blockf("refused")
}
`,
			clean: `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
	task.Block("refused")
}
`,
			wantInSuggestion: `task.Block("refused")`,
		},
		{
			name: "Warn",
			dirty: `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
	task.Warn("stale entry ignored")
}
`,
			clean: `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
	task.Problem("stale entry ignored", evo.Severity(evo.SeverityWarning))
}
`,
			wantInSuggestion: `task.Problem("stale entry ignored", evo.Severity(evo.SeverityWarning))`,
		},
		{
			name: "Step",
			dirty: `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
	task.Step(1, 3, "file.go")
}
`,
			clean: `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
	task.Progress(1, 3).Doing("file.go")
}
`,
			wantInSuggestion: `task.Progress(1, 3).Doing("file.go")`,
		},
		{
			name: "Kept",
			dirty: `package p
import evo "github.com/zachbornheimer/evident-output"
var reasonProtected = evo.Reason("protected")
func f(task *evo.TaskHandle) {
	task.Kept(reasonProtected)
}
`,
			clean: `package p
import evo "github.com/zachbornheimer/evident-output"
var reasonProtected = evo.Reason("protected")
func f(task *evo.TaskHandle) {
	task.Skipped(reasonProtected)
}
`,
			wantInSuggestion: `task.Skipped(reasonProtected)`,
		},
		{
			name: "Evidence",
			dirty: `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
	_ = task.Evidence()
}
`,
			clean: `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
	_ = task.Capture()
}
`,
			wantInSuggestion: `task.Capture()`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := review.GoSource(tc.name+".go", tc.dirty)
			found := findAPI032(res)
			if len(found) == 0 {
				t.Fatalf("dirty %s: want API-032, got %+v", tc.name, res.Findings)
			}
			var hit review.Finding
			for _, f := range found {
				if f.Suggestion != "" && strings.Contains(f.Suggestion, tc.wantInSuggestion) {
					hit = f
					break
				}
			}
			if hit.Suggestion == "" {
				t.Fatalf("dirty %s: no API-032 suggestion containing %q; got %q",
					tc.name, tc.wantInSuggestion, joinSuggestions(found))
			}
			if _, ok := rules.Explain(hit.RuleID); !ok {
				t.Fatalf("rules.Explain(%q) failed", hit.RuleID)
			}
			fixed := applyReplace(t, tc.dirty, hit.Suggestion)
			again := review.GoSource(tc.name+".go", fixed)
			if again.RecheckRequired || len(again.Findings) != 0 {
				t.Fatalf("applied rewrite still dirty: recheck=%v findings=%+v\nfixed:\n%s",
					again.RecheckRequired, again.Findings, fixed)
			}
			clean := review.GoSource(tc.name+".go", tc.clean)
			if clean.RecheckRequired || len(clean.Findings) != 0 {
				t.Fatalf("canonical rewrite is dirty: recheck=%v findings=%+v",
					clean.RecheckRequired, clean.Findings)
			}
			for _, f := range findAPI032(review.GoSourceAt(tc.name+".go", tc.dirty, "v1.0.0")) {
				if strings.Contains(f.Message, "removed in 1.1") {
					t.Fatalf("v1.0.0 pin must not see the 1.1 %s rewrite: %+v", tc.name, f)
				}
			}
		})
	}
}

func TestAPI032_CaptureStaysSilent(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(task *evo.TaskHandle) {
	_ = task.Capture()
}
`
	res := review.GoSource("capture.go", src)
	for _, f := range findAPI032(res) {
		t.Fatalf("Capture is live 1.1 API and must not fire API-032: %+v", f)
	}
	if res.RecheckRequired {
		t.Fatalf("Capture fixture must be clean: %+v", res.Findings)
	}
}
