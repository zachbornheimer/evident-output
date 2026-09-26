package ledger

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/wire"
)

func TestTenseStrings(t *testing.T) {
	cases := []struct {
		name          string
		tense         Tense
		wantString    string
		wantDeclared  string
		wantRecorded  string
		wantWireEvent string
	}{
		{"changed", Changed, "changed", "changes.declared", "change.recorded", wire.EventEffectCommitted},
		{"planned", Planned, "planned", "plan.declared", "plan.recorded", wire.EventEffectPlanned},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.tense.String(); got != c.wantString {
				t.Errorf("String() = %q, want %q", got, c.wantString)
			}
			if got := c.tense.DeclaredEvent(); got != c.wantDeclared {
				t.Errorf("DeclaredEvent() = %q, want %q", got, c.wantDeclared)
			}
			if got := c.tense.RecordedEvent(); got != c.wantRecorded {
				t.Errorf("RecordedEvent() = %q, want %q", got, c.wantRecorded)
			}
			if got := c.tense.WireEvent(); got != c.wantWireEvent {
				t.Errorf("WireEvent() = %q, want %q", got, c.wantWireEvent)
			}
		})
	}
}

func TestTenseVerb(t *testing.T) {
	if got := Planned.Verb("delete"); got != "delete" {
		t.Errorf("Planned.Verb(delete) = %q, want %q", got, "delete")
	}
	if got := Changed.Verb("delete"); got != "deleted" {
		t.Errorf("Changed.Verb(delete) = %q, want %q", got, "deleted")
	}
}

func TestTenseFor(t *testing.T) {
	if got := TenseFor(true); got != Planned {
		t.Errorf("TenseFor(true) = %v, want Planned", got)
	}
	if got := TenseFor(false); got != Changed {
		t.Errorf("TenseFor(false) = %v, want Changed", got)
	}
}
