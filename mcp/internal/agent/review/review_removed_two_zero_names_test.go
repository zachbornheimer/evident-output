package review_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/vocabulary"
)

// removedName is one name the vocabulary classes as removed: a dirty 1.x use
// of it, and the text API-032 must give for it. An empty want means the
// finding carries the retired table's own replacement.
type removedName struct {
	dirty, want string
}

func vocabularyPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "testdata", "api_vocabulary.txt")
}

// TestAPI032_EveryRemovedNameLandsOnTwoZero requires every name the
// vocabulary lists as removed to have a dirty use that review flags with its
// 2.0 replacement, and no fixture for a name the vocabulary does not remove.
func TestAPI032_EveryRemovedNameLandsOnTwoZero(t *testing.T) {
	entries, err := vocabulary.LoadVocabulary(vocabularyPath(t))
	if err != nil {
		t.Fatal(err)
	}
	fixtures := removedNameFixtures()
	removed := map[string]bool{}
	for _, e := range entries {
		if e.Class != vocabulary.ClassRemoved {
			continue
		}
		removed[e.Name] = true
		fx, ok := fixtures[e.Name]
		if !ok {
			t.Errorf("missing dirty fixture for removed name %s", e.Name)
			continue
		}
		t.Run(e.Name, func(t *testing.T) { checkRemovedName(t, e.Name, fx) })
	}
	if len(removed) == 0 {
		t.Fatal("vocabulary loaded no class=removed names")
	}
	for name := range fixtures {
		if !removed[name] {
			t.Errorf("fixture %s is not a removed name in the vocabulary", name)
		}
	}
}

func checkRemovedName(t *testing.T, name string, fx removedName) {
	t.Helper()
	res := review.GoSource(name+".go", fx.dirty)
	method := name[strings.LastIndex(name, ".")+1:]
	want := fx.want
	if want == "" {
		want = tableReplacement(t, name)
	}
	var hit *review.Finding
	for _, f := range findAPI032(res) {
		if strings.Contains(f.Message, method) && strings.Contains(f.Suggestion+" "+f.Message, want) {
			hit = &f
			break
		}
	}
	if hit == nil {
		t.Fatalf("no API-032 finding about %s carrying %q; got %+v", name, want, res.Findings)
	}
	if hits := rules.UnexplainedRetired(hit.Suggestion + " " + hit.Message); len(hits) > 0 {
		t.Errorf("finding teaches retired %s: %+v", hits[0].Symbol.Contract, hit)
	}
	if _, ok := rules.Explain(hit.RuleID); !ok {
		t.Errorf("rules.Explain(%q) failed", hit.RuleID)
	}
}

func tableReplacement(t *testing.T, name string) string {
	t.Helper()
	for _, s := range rules.RetiredSymbols() {
		if s.Contract == name || s.Contract == name+"(" {
			return s.Replacement
		}
	}
	t.Fatalf("%s has no retired-table replacement and no explicit want", name)
	return ""
}

func removedNameFixtures() map[string]removedName {
	m := map[string]removedName{
		"TaskHandle.Failf":  {evoBody("func f(task *evo.TaskHandle) {\n\ttask.Failf(\"boom\")\n}\n"), `return evo.Fail("boom")`},
		"Output.Failf":      {evoBody("func f(out *evo.Output) {\n\tout.Failf(\"boom\")\n}\n"), "return evo.Fail(summary)"},
		"TaskHandle.Blockf": {evoBody("func f(task *evo.TaskHandle) {\n\ttask.Blockf(\"refused\")\n}\n"), `return evo.Refuse("refused")`},
		"Warn":              {evoBody("func f() {\n\tevo.Warn(\"stale entry ignored\")\n}\n"), ""},
		"Output.Warn":       {evoBody("func f(out *evo.Output) {\n\tout.Warn(\"stale entry ignored\")\n}\n"), `<task>.Problem("stale entry ignored")`},
		"TaskHandle.Warn":   {evoBody("func f(task *evo.TaskHandle) {\n\ttask.Warn(\"stale entry ignored\")\n}\n"), `task.Problem("stale entry ignored")`},
		"TaskHandle.Step":   {evoBody("func f(task *evo.TaskHandle) {\n\ttask.Step(1, 3, \"file.go\")\n}\n"), `task.Progress(1, 3).Doing("file.go")`},
		"TaskHandle.Kept": {
			evoBody("func f(task *evo.TaskHandle, reason string) {\n\ttask.Kept(reason)\n}\n"),
			`return evo.Exclude(reason)`,
		},
		"Evidence":               {evoBody("func f(task *evo.TaskHandle) {\n\t_ = task.Evidence()\n}\n"), `task.Writer()`},
		"Failure.Next":           {evoBody("func f(fail *evo.Failure) {\n\tfail.Next(evo.Label(\"retry\"))\n}\n"), "evo.Next(evo.Action{Label:"},
		"Failure.NextCommand":    {evoBody("func f(fail *evo.Failure) {\n\tfail.NextCommand(\"git\", \"status\")\n}\n"), `evo.Next(evo.Action{Command: []string{"git", "status"}})`},
		"TaskHandle.Next":        {evoBody("func f(task *evo.TaskHandle) {\n\ttask.Next(evo.Label(\"retry\"))\n}\n"), "evo.Next(evo.Action{Label:"},
		"TaskHandle.NextCommand": {evoBody("func f(task *evo.TaskHandle) {\n\ttask.NextCommand(\"git\", \"status\")\n}\n"), `evo.Next(evo.Action{Command: []string{"git", "status"}})`},
		"Output.Next":            {evoBody("func f(out *evo.Output) {\n\tout.Next(evo.Label(\"retry\"))\n}\n"), "evo.Next(evo.Action{Label:"},
		"Output.NextCommand":     {evoBody("func f(out *evo.Output) {\n\tout.NextCommand(\"git\", \"status\")\n}\n"), `evo.Next(evo.Action{Command: []string{"git", "status"}})`},
		"EncodeJSON":             {evoIOBody("func f(w io.Writer, r evo.Result) {\n\t_ = evo.EncodeJSON(w, r)\n}\n"), ""},
		"EncodeJSONL":            {evoIOBody("func f(w io.Writer, r evo.Result) {\n\t_ = evo.EncodeJSONL(w, r)\n}\n"), ""},
		"EncodeEventJSON":        {evoIOBody("func f(w io.Writer, r evo.Result) {\n\t_ = evo.EncodeEventJSON(w, r)\n}\n"), ""},
		"ForSkip":                {evoBody("func f() {\n\t_ = evo.ForSkip()\n}\n"), ""},
		"OnTask":                 {evoBody("func f() {\n\t_ = evo.OnTask()\n}\n"), ""},
		"EventSchemaVersion":     {evoBody("func f() {\n\t_ = evo.EventSchemaVersion\n}\n"), ""},
	}
	addOptionNames(m)
	addExportNames(m)
	return m
}

// addOptionNames covers the Option constructors; each lands on the 2.0
// Config or Facades field, or on the reason it has none.
func addOptionNames(m map[string]removedName) {
	type option struct{ name, params, call, want string }
	for _, o := range []option{
		{"To", "w io.Writer", "evo.To(w)", "Facades: evo.Facades{Stdout: w}"},
		{"AlsoWrite", "w io.Writer", "evo.AlsoWrite(w)", "io.MultiWriter(os.Stdout, w)"},
		{"NoColor", "", "evo.NoColor()", "NO_COLOR"},
		{"Stdin", "r io.Reader", "evo.Stdin(r)", "Facades: evo.Facades{Stdin: r}"},
		{"DryRun", "", "evo.DryRun()", "DryRun: true"},
		{"VisibilityDelay", "", "evo.VisibilityDelay(0)", "no 2.0 equivalent"},
		{"Diagnostics", "w io.Writer", "evo.Diagnostics(w)", "Facades: evo.Facades{Stderr: w}"},
		{"Title", "", `evo.Title("tool")`, `Name: "tool"`},
		{"ResultStream", "w io.Writer", "evo.ResultStream(w)", "Run.ResultWriter()"},
		{"MaxFrameRate", "", "evo.MaxFrameRate(20)", "no 2.0 equivalent"},
		{"Width", "", "evo.Width(80)", "no 2.0 equivalent"},
		{"Redact", "r evo.Redactor", "evo.Redact(r)", "Facades: evo.Facades{Redactor: r}"},
		{"Runner", "r evo.Process", "evo.Runner(r)", "Facades: evo.Facades{Process: r}"},
		{"MaxEntities", "", "evo.MaxEntities(10)", "no 2.0 equivalent"},
		{"MaxEvents", "", "evo.MaxEvents(10)", "no 2.0 equivalent"},
		{"Strict", "", "evo.Strict()", "no 2.0 equivalent"},
		{"Glyphs", "", "evo.Glyphs(1)", "no 2.0 equivalent"},
		{"DataProjection", "", "evo.DataProjection()", "Run.ResultWriter()"},
		{"ExternalProjection", "", "evo.ExternalProjection()", "Run.ResultWriter()"},
		{"DebugLevel", "", "evo.DebugLevel(0)", "Config.Debug"},
		{"DebugAddSource", "", "evo.DebugAddSource()", "Config.Debug"},
		{"DebugHistory", "", "evo.DebugHistory()", "Config.Debug"},
		{"DebugPane", "", "evo.DebugPane()", "Config.Debug"},
	} {
		m[o.name] = removedName{evoIOBody("func f(" + o.params + ") {\n\t_ = " + o.call + "\n}\n"), o.want}
	}
}

// addExportNames covers the removed exports the retired table maps: types
// and values used by name, and the 1.2 file-tree nouns.
func addExportNames(m map[string]removedName) {
	for _, name := range []string{
		"EvidenceOption", "EvidenceStream", "EvidenceStreamCombined", "EvidenceStreamStdout", "EvidenceStreamStderr",
		"MaxEvidenceBytes", "Option", "ReasonOption", "ErrReasonSkipOnly", "ErrReasonWrongTask", "ConclusionJSON", "EventJSON",
		"JSONAction", "JSONChanges", "JSONCollection", "JSONCommand", "JSONDocument", "JSONEffectRecord", "JSONMessage",
		"JSONOutputMeta", "JSONPlan", "JSONProblem", "JSONProgress", "JSONSchemaVersion", "JSONTask",
		"Tree", "Find", "Download", "Extract", "Clone", "Checksum",
	} {
		m[name] = removedName{evoBody("func f() {\n\t_ = evo." + name + "\n}\n"), ""}
	}
}
