package engine

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// durableRecorder is an interactive surface that keeps the durable text in
// the order it reached the terminal.
type durableRecorder struct {
	paintRecorder
	durableMu sync.Mutex
	durable   []string
}

func (s *durableRecorder) WriteDurable(text string) {
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	s.durable = append(s.durable, text)
}

func (s *durableRecorder) durableText() []string {
	s.durableMu.Lock()
	defer s.durableMu.Unlock()
	return append([]string(nil), s.durable...)
}

// indexOfDurable is the position of the first durable write containing every
// fragment, or -1.
func indexOfDurable(writes []string, fragments ...string) int {
	for i, write := range writes {
		matches := true
		for _, fragment := range fragments {
			matches = matches && strings.Contains(write, fragment)
		}
		if matches {
			return i
		}
	}
	return -1
}

// A Suspend window owns the terminal: a sibling that settles inside it must
// not print its row between the Confirm prompt and the answer. The gate's own
// rows still print inside the window; the sibling's waits for it to close.
func TestSiblingRowSettledInsideAConfirmWindowPrintsAfterTheGateRows(t *testing.T) {
	answers, typed, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = answers.Close(); _ = typed.Close() })
	surface := &durableRecorder{}
	out := newOutput("job", withTerminal(surface), visibilityDelay(0), withNoColor(), stdin(answers), maxConcurrency(4))

	release := make(chan struct{})
	sibling := out.Task("sibling").Define(func(context.Context) error {
		<-release
		return errors.New("sibling broke")
	})
	go func() {
		for indexOfDurable(surface.durableText(), "go?", "[y/N]") < 0 {
			time.Sleep(time.Millisecond)
		}
		close(release)
		_ = sibling.Wait()
		_, _ = typed.WriteString("y\n")
	}()

	if !out.Confirm("go?") {
		t.Fatal("Confirm = false, want the typed yes")
	}
	_ = out.Finish()

	writes := surface.durableText()
	prompt := indexOfDurable(writes, "go?", "[y/N]")
	gateRow := -1
	for i, write := range writes {
		if i > prompt && strings.Contains(write, "go?") && !strings.Contains(write, "[y/N]") {
			gateRow = i
			break
		}
	}
	siblingRow := indexOfDurable(writes, "sibling")
	if prompt < 0 || gateRow < 0 || siblingRow < 0 {
		t.Fatalf("durable writes lack prompt=%d gate row=%d sibling row=%d: %q", prompt, gateRow, siblingRow, writes)
	}
	if prompt >= gateRow || gateRow >= siblingRow {
		t.Fatalf("durable order = prompt %d, gate row %d, sibling row %d; want the sibling after the gate rows: %q",
			prompt, gateRow, siblingRow, writes)
	}
}
