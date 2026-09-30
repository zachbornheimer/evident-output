//go:build unix

package freshness_test

import (
	"path/filepath"
	"testing"
)

func TestC02_050_ExecWithoutDeclaredOutputsAlwaysRuns(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	always := stage{name: "always", script: "true"}
	runStages(t, state, always)
	if !runStages(t, state, always)["always"] {
		t.Fatal("an Exec with no declared Outputs was skipped")
	}
}
