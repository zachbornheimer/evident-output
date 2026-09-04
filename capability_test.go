package evo

import "testing"

// capability_test.go is an internal (package evo) test file — C8
// unexported ColorLevel/CapabilityProfile/DetectCapabilities, so this
// coverage moved from evo_test to exercise them directly. It can't import
// testkit here (testkit imports evo — an internal evo test importing it
// back would be a cycle), so fakeCapabilityScreen is a minimal inline
// LiveSurface stand-in instead.

type fakeCapabilityScreen struct {
	width, height int
}

func (f *fakeCapabilityScreen) ID() string          { return "fake-capability-screen" }
func (f *fakeCapabilityScreen) Columns() int        { return f.width }
func (f *fakeCapabilityScreen) Rows() int           { return f.height }
func (f *fakeCapabilityScreen) IsInteractive() bool { return true }
func (f *fakeCapabilityScreen) WriteLive(string)    {}
func (f *fakeCapabilityScreen) ClearLive()          {}
func (f *fakeCapabilityScreen) WriteDurable(string) {}
func (f *fakeCapabilityScreen) WriteFinal(string)   {}

func TestCapability_NoColorForcesNone(t *testing.T) {
	p := detectCapabilities(NoColor(), Plain())
	if p.Color != colorNone {
		t.Fatal(p.Color)
	}
	if p.Interactive {
		t.Fatal("plain should not be interactive")
	}
}

func TestCapability_FromScreen(t *testing.T) {
	s := &fakeCapabilityScreen{width: 100, height: 40}
	p := detectCapabilities(Terminal(s), VisibilityDelay(0))
	if p.Width != 100 || p.Height != 40 {
		t.Fatalf("%+v", p)
	}
	if !p.Interactive {
		t.Fatal("expected interactive")
	}
}
