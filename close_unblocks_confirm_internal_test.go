package evo

import (
	"bytes"
	"os"
	"testing"
	"time"
)

// A Confirm waiting on a pipe nobody writes to would hold its caller forever
// once the run is closed. Close ends the wait: the gate is Cancelled and
// Confirm returns false.
func TestSlice36_CloseUnblocksAConfirmWaitingOnAPipe(t *testing.T) {
	answers, typed, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = answers.Close(); _ = typed.Close() })
	surface := &durableRecorder{}
	out := newOutput("job", to(&bytes.Buffer{}), withTerminal(surface), visibilityDelay(0), withNoColor(), stdin(answers))

	confirmed := make(chan bool, 1)
	go func() { confirmed <- out.Confirm("go?") }()
	for indexOfDurable(surface.durableText(), "go?", "[y/N]") < 0 {
		time.Sleep(time.Millisecond)
	}

	_ = out.Close()

	select {
	case yes := <-confirmed:
		if yes {
			t.Error("Confirm returned true after Close with no answer")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Confirm never returned after Close")
	}
}
