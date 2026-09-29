package apisurface

import (
	"strings"
	"testing"
)

func TestParseVocabulary_ReadsClassesAndSkipsComments(t *testing.T) {
	entries, err := ParseVocabulary([]byte("" +
		"# comment\n" +
		"\n" +
		"Init\tcanonical\tRun\n" +
		"Delay\thelper\tRun\n" +
		"AlsoWrite\tremoved\tRun\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	want := []Entry{
		{Name: "Init", Class: ClassCanonical, Concept: "Run"},
		{Name: "Delay", Class: ClassHelper, Concept: "Run"},
		{Name: "AlsoWrite", Class: ClassRemoved, Concept: "Run"},
	}
	for i, e := range entries {
		if e != want[i] {
			t.Errorf("entries[%d] = %+v, want %+v", i, e, want[i])
		}
	}
}

func TestParseVocabulary_RejectsUnknownClass(t *testing.T) {
	_, err := ParseVocabulary([]byte("Foo\tsugar\tRun\n"))
	if err == nil || !strings.Contains(err.Error(), "unknown class") {
		t.Fatalf("err = %v, want unknown class", err)
	}
}

func TestParseVocabulary_RejectsDuplicateName(t *testing.T) {
	_, err := ParseVocabulary([]byte("Init\tcanonical\tRun\nInit\thelper\tRun\n"))
	if err == nil || !strings.Contains(err.Error(), "duplicate name") {
		t.Fatalf("err = %v, want duplicate name", err)
	}
}

func TestParseVocabulary_RejectsMalformedRow(t *testing.T) {
	_, err := ParseVocabulary([]byte("Init canonical Run\n"))
	if err == nil || !strings.Contains(err.Error(), "want name") {
		t.Fatalf("err = %v, want field-count error", err)
	}
}

func TestIdent_WalkLines(t *testing.T) {
	cases := []struct {
		line, want string
	}{
		{"type Option", "Option"},
		{"type Config.Stdout", "Config.Stdout"},
		{"value EventSchemaVersion", "EventSchemaVersion"},
		{"func AlsoWrite(w io.Writer)  Option", "AlsoWrite"},
		{"func (Output) Cancel(reason string)", "Output.Cancel"},
		{"func (Failure) Error()  string", "Failure.Error"},
		{"func (TaskHandle) After(preds ...any)  *TaskHandle", "TaskHandle.After"},
		{"not a walk line", ""},
	}
	for _, c := range cases {
		if got := Ident(c.line); got != c.want {
			t.Errorf("Ident(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}

func TestCheckVocabulary_RemovedPresentAndUnclassified(t *testing.T) {
	entries := []Entry{
		{Name: "Init", Class: ClassCanonical, Concept: "Run"},
		{Name: "AlsoWrite", Class: ClassRemoved, Concept: "Run"},
		{Name: "Delay", Class: ClassHelper, Concept: "Run"},
	}
	surface := []string{
		"func Init(configs ...Config)  *Output",
		"func AlsoWrite(w io.Writer)  Option",
		"func Mystery()",
	}
	got := CheckVocabulary(surface, entries)
	if got.OK() {
		t.Fatal("want a failing report")
	}
	if len(got.RemovedPresent) != 1 || got.RemovedPresent[0] != "AlsoWrite" {
		t.Errorf("RemovedPresent = %v, want [AlsoWrite]", got.RemovedPresent)
	}
	if len(got.Unclassified) != 1 || got.Unclassified[0] != "Mystery" {
		t.Errorf("Unclassified = %v, want [Mystery]", got.Unclassified)
	}
	if len(got.Missing) != 0 {
		t.Errorf("helper Delay must not be required on the surface, Missing = %v", got.Missing)
	}
}

func TestCheckVocabulary_CanonicalMissingFails(t *testing.T) {
	entries := []Entry{
		{Name: "Init", Class: ClassCanonical, Concept: "Run"},
		{Name: "Delay", Class: ClassHelper, Concept: "Run"},
	}
	got := CheckVocabulary(nil, entries)
	if got.OK() {
		t.Fatal("want failure when canonical Init is absent")
	}
	if len(got.Missing) != 1 || got.Missing[0] != "Init" {
		t.Errorf("Missing = %v, want [Init]", got.Missing)
	}
}

func TestCheckVocabulary_OKWhenSurfaceMatchesFreeze(t *testing.T) {
	entries := []Entry{
		{Name: "Init", Class: ClassCanonical, Concept: "Run"},
		{Name: "AlsoWrite", Class: ClassRemoved, Concept: "Run"},
	}
	surface := []string{"func Init(configs ...Config)  *Output"}
	got := CheckVocabulary(surface, entries)
	if !got.OK() {
		t.Fatalf("report not OK:\n%s", got)
	}
}
