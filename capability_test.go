package evo_test

import (
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestCapability_NoColorForcesNone(t *testing.T) {
	p := evo.DetectCapabilities(evo.NoColor(), evo.Plain())
	if p.Color != evo.ColorNone {
		t.Fatal(p.Color)
	}
	if p.Interactive {
		t.Fatal("plain should not be interactive")
	}
}

func TestCapability_FromScreen(t *testing.T) {
	s := testkit.NewScreen(testkit.Interactive(), testkit.Width(100), testkit.Height(40))
	p := evo.DetectCapabilities(evo.Terminal(s))
	if p.Width != 100 || p.Height != 40 {
		t.Fatalf("%+v", p)
	}
	if !p.Interactive {
		t.Fatal("expected interactive")
	}
}
