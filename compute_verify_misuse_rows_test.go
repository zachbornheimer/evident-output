package evo

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestComputeVerifyMisuseRowsAreAccurate pins that declaring Compute and Verify on one Task prints the
// misuse remedy and a truthful "not started" row: the Task never ran, so no "incomplete" row blames a missing
// finish and no next step tells the caller to give the Task a Define callback they already wrote.
func TestComputeVerifyMisuseRowsAreAccurate(t *testing.T) {
	var human bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &human}}, tuning{color: colorNever})
	defer func() { _ = out.close() }()
	made := out.Task("c").Verify(func(context.Context) (bool, error) { return false, nil })
	Compute(made, func(context.Context) (int, error) { return 1, nil })

	_ = out.finish()

	got := human.String()
	if !strings.Contains(got, "Compute") {
		t.Fatalf("the misuse remedy is missing:\n%s", got)
	}
	if !strings.Contains(got, "- c  not started") {
		t.Errorf("the refused Task does not read as not started:\n%s", got)
	}
	for _, misleading := range []string{"incomplete", "run concluded before finish", "a Define callback that returns"} {
		if strings.Contains(got, misleading) {
			t.Errorf("output carries %q, which does not apply to Compute+Verify misuse:\n%s", misleading, got)
		}
	}
}
