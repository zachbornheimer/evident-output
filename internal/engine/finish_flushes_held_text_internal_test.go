package engine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

// finalRecorder is a durable-text surface that also keeps the final frame.
type finalRecorder struct {
	durableRecorder
	finalMu sync.Mutex
	final   []string
}

func (s *finalRecorder) WriteFinal(text string) {
	s.finalMu.Lock()
	defer s.finalMu.Unlock()
	s.final = append(s.final, text)
}

// A sibling's failed row is held while a Confirm window is open. Finish must
// print it, because nothing else will once the run is over.
func TestSlice36_FinishFlushesTextHeldBehindOpenConfirm(t *testing.T) {
	answers, typed, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = answers.Close(); _ = typed.Close() })
	surface := &finalRecorder{}
	var primary bytes.Buffer
	out := newOutput("job", to(&primary), withTerminal(surface), visibilityDelay(0), withNoColor(), stdin(answers), maxConcurrency(4))

	sibling := out.Task("sibling").Define(func(context.Context) error { return errors.New("sibling broke") })
	confirmed := make(chan bool, 1)
	go func() { confirmed <- out.Confirm("go?") }()
	for indexOfDurable(surface.durableText(), "go?", "[y/N]") < 0 {
		time.Sleep(time.Millisecond)
	}
	_ = sibling.Wait()
	time.Sleep(20 * time.Millisecond)
	if _, finished := within(3*time.Second, out.Finish); !finished {
		t.Fatal("Finish did not return")
	}
	if indexOfDurable(surface.durableText(), "sibling") < 0 {
		t.Errorf("sibling's failed row was not printed by the time Finish returned: durable=%q", surface.durableText())
	}
	_ = out.Close()
	select {
	case <-confirmed:
	case <-time.After(3 * time.Second):
		t.Log("Confirm never returned after Close")
	}
}
