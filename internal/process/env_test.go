package process_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/process"
)

func TestEnvReadsTheNamedVariableAndNothingElse(t *testing.T) {
	t.Setenv("EVO_PROCESS_PROBE", "value")
	if got := process.Env("EVO_PROCESS_PROBE"); got != "value" {
		t.Fatalf("Env(set) = %q, want %q", got, "value")
	}
	if got := process.Env("EVO_PROCESS_PROBE_UNSET"); got != "" {
		t.Fatalf("Env(unset) = %q, want empty", got)
	}
}

func TestEnvironListsEverySetVariableAsNameValue(t *testing.T) {
	t.Setenv("EVO_PROCESS_PROBE", "value")
	if !slices.Contains(process.Environ(), "EVO_PROCESS_PROBE=value") {
		t.Fatalf("Environ() lacks EVO_PROCESS_PROBE=value")
	}
}

func TestArgsStartsWithTheProgramName(t *testing.T) {
	got := process.Args()
	if len(got) == 0 || got[0] != os.Args[0] {
		t.Fatalf("Args() = %v, want it to start with %q", got, os.Args[0])
	}
}

func TestLookPathFindsAnExecutableOnPathAndFailsForAMissingOne(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "evo-process-tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	if got, err := process.LookPath("evo-process-tool"); err != nil || got != tool {
		t.Fatalf("LookPath(present) = %q, %v; want %q", got, err, tool)
	}
	if got, err := process.LookPath("evo-process-missing"); err == nil {
		t.Fatalf("LookPath(missing) = %q, nil; want an error", got)
	}
}

func TestIsExplicitPathTellsAPathFromABareName(t *testing.T) {
	cases := map[string]bool{
		"go":                                    false,
		"":                                      false,
		"./go":                                  true,
		"bin" + string(os.PathSeparator) + "go": true,
	}
	for executable, want := range cases {
		if got := process.IsExplicitPath(executable); got != want {
			t.Errorf("IsExplicitPath(%q) = %v, want %v", executable, got, want)
		}
	}
}

func TestNoopRedactorLeavesTextUnchanged(t *testing.T) {
	const text = "token=s3kr3t"
	if got := (process.NoopRedactor{}).RedactString(text); got != text {
		t.Fatalf("RedactString(%q) = %q, want it unchanged", text, got)
	}
}
