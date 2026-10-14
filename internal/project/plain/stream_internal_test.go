package plain

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// quietRegion is a Region with no live surface. While windowOpen it holds
// text as a window would.
type quietRegion struct {
	windowOpen bool
	held       []string
}

func (r *quietRegion) Interactive() bool { return false }
func (r *quietRegion) Hold(text string) bool {
	if r.windowOpen {
		r.held = append(r.held, text)
	}
	return r.windowOpen
}

func (r *quietRegion) Flush(emit func(string)) {
	for _, text := range r.held {
		emit(text)
	}
	r.held = nil
}
func (r *quietRegion) Durable(string) bool { return false }
func (r *quietRegion) Refresh()            {}
func (r *quietRegion) Signal(bool)         {}

// bufferSinks collect human text.
type bufferSinks struct{ human strings.Builder }

func (s *bufferSinks) Human(text string)      { s.human.WriteString(text) }
func (s *bufferSinks) Diagnostic(string) bool { return false }
func (s *bufferSinks) Separate() bool         { return false }
func (s *bufferSinks) SharesTerminal() bool   { return false }

func TestWriteCountsEveryRowAndHoldsInsideAWindow(t *testing.T) {
	region, sinks := &quietRegion{}, &bufferSinks{}
	s := NewStream(Config{Run: record.NewRun(), Region: region, Sinks: sinks})

	s.Write("first\n")
	region.windowOpen = true
	s.Write("held\n")
	s.WriteOwner("prompt\n")
	region.windowOpen = false
	s.Flush()

	if got, want := sinks.human.String(), "first\nprompt\nheld\n"; got != want {
		t.Fatalf("human stream = %q, want %q", got, want)
	}
	if s.Written() != 3 {
		t.Fatalf("Written() = %d, want 3", s.Written())
	}
}

func TestSuppressedHumanStillCountsRows(t *testing.T) {
	sinks := &bufferSinks{}
	s := NewStream(Config{Run: record.NewRun(), Region: &quietRegion{}, Sinks: sinks, SuppressHuman: true})

	s.Write("row\n")

	if sinks.human.Len() != 0 || s.Written() != 1 {
		t.Fatalf("human %q, written %d; want nothing written and one row counted", sinks.human.String(), s.Written())
	}
}

func TestMilestonesThinProgressButKeepFirstAndLast(t *testing.T) {
	var m mark
	const total = 100
	streamed := 0
	for completed := int64(1); completed <= total; completed++ {
		if m.crossesMilestone(completed, total) {
			streamed++
			m.progressStarted, m.progressEmitted = true, completed
		}
	}
	if streamed != progressMilestones+1 {
		t.Fatalf("streamed %d ticks of %d, want first plus %d milestones", streamed, total, progressMilestones)
	}
	if m.progressEmitted != total {
		t.Fatalf("last streamed tick = %d, want the final %d", m.progressEmitted, total)
	}
}

func TestStreamNameQualifiesAChildByItsGroup(t *testing.T) {
	cases := []struct {
		row  Declared
		want string
	}{
		{Declared{Name: "build"}, "build"},
		{Declared{Name: "classify", Group: "repo a", InGroup: true}, "repo a  classify"},
		{Declared{Name: "deploy", Group: "deploy", InGroup: true}, "deploy"},
	}
	for _, c := range cases {
		if got := (&row{Declared: c.row}).streamName(); got != c.want {
			t.Errorf("streamName(%+v) = %q, want %q", c.row, got, c.want)
		}
	}
}
