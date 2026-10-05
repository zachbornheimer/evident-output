package evo_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/wireschema"
)

// factsAndKeptRun is a run whose human output hides a Task Fact, a run
// Fact, and a kept child's reason by default (§13/§21).
func factsAndKeptRun(t *testing.T, mode string) string {
	t.Helper()
	withLookupEnv(t, map[string]string{"EVO_OUTPUT": mode})
	var buf bytes.Buffer
	out := isolatedInit(t, evo.Config{Stdout: &buf, Stderr: io.Discard})
	out.Fact("language", "go")
	out.Task("measure").Fact("on disk", "8.0 KB").Define(func(context.Context) error { return nil })
	g := out.Group("branches")
	g.Task("main").Skipped(evo.Reason("protected"))
	g.Task("merged1").Skipped(evo.Reason("merged"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// TestEVOOutput_JSONCarriesFactsAndDispositions pins E-090: the
// EVO_OUTPUT=json document dropped Task and run Facts and every Kept /
// Skipped record, so a machine consumer lost what human verbosity hides.
// Contract §13: JSON/JSONL retain structured Facts regardless of human
// verbosity. EVO_OUTPUT=json now selects the evo.run document.
func TestEVOOutput_JSONCarriesFactsAndDispositions(t *testing.T) {
	got := factsAndKeptRun(t, "json")
	schema, err := os.ReadFile("schema/run.v2.json")
	if err != nil {
		t.Fatalf("read schema/run.v2.json: %v", err)
	}
	strict, err := wireschema.Strict(schema)
	if err != nil {
		t.Fatalf("wireschema.Strict: %v", err)
	}
	if err := wireschema.Validate(strict, []byte(got)); err != nil {
		t.Errorf("EVO_OUTPUT=json document does not conform to schema/run.v2.json: %v", err)
	}
	for _, want := range []string{`"on disk"`, `"8.0 KB"`, `"language"`, `"protected"`, `"merged"`} {
		if !strings.Contains(got, want) {
			t.Errorf("EVO_OUTPUT=json lacks %s:\n%s", want, got)
		}
	}
}

// TestEVOOutput_JSONLCarriesFactsAndDispositions is E-090 for the event
// lines.
func TestEVOOutput_JSONLCarriesFactsAndDispositions(t *testing.T) {
	got := factsAndKeptRun(t, "jsonl")
	for _, want := range []string{`"on disk"`, `"8.0 KB"`, `"language"`, `"protected"`, `"merged"`} {
		if !strings.Contains(got, want) {
			t.Errorf("EVO_OUTPUT=jsonl lacks %s:\n%s", want, got)
		}
	}
}
