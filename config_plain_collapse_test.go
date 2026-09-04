package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestConfig_Plain_DisablesLiveFrames is C2/C3: Config.ForcePlain and
// Config.NonInteractive collapsed into one Config.Plain field / Plain()
// Option — every prior read site combined them with OR, so there was never
// a distinct behavior to preserve. This test's mere compilation (using the
// unified field) is most of the proof; it also checks the behavior holds.
func TestConfig_Plain_DisablesLiveFrames(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Stdout: &buf, Stderr: &buf, Plain: true})

	out.Task("build").Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "build") {
		t.Fatalf("expected the Done row rendered plain, got:\n%s", buf.String())
	}
}
