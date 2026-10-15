package evo

import (
	"bytes"
	"os"
	"testing"
	"time"
)

// confirmQuietPeriod is how long the test lets a second prompt show itself
// before concluding it is waiting its turn.
const confirmQuietPeriod = 100 * time.Millisecond

// One Confirm owns the terminal at a time. A second one asked while the first
// window is open waits for it to close: its prompt would otherwise land inside
// the first window, and both would read the same stdin.
func TestSlice36_ASecondConfirmWaitsForTheFirstWindowToClose(t *testing.T) {
	answers, typed, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = answers.Close(); _ = typed.Close() })
	surface := &durableRecorder{}
	out := newOutput("job", to(&bytes.Buffer{}), withTerminal(surface), visibilityDelay(0), withNoColor(), stdin(answers))
	t.Cleanup(func() { _ = out.close() })

	first, second := make(chan bool, 1), make(chan bool, 1)
	go func() { first <- out.Confirm("first?") }()
	for indexOfDurable(surface.durableText(), "first?", "[y/N]") < 0 {
		time.Sleep(time.Millisecond)
	}
	go func() { second <- out.Confirm("second?") }()
	time.Sleep(confirmQuietPeriod)

	if indexOfDurable(surface.durableText(), "second?") >= 0 {
		t.Fatalf("the second prompt was written inside the first window: %q", surface.durableText())
	}

	answer(t, typed, "y\n")
	if !receive(t, first, "first Confirm") {
		t.Error("first Confirm = false, want true")
	}
	for indexOfDurable(surface.durableText(), "second?", "[y/N]") < 0 {
		time.Sleep(time.Millisecond)
	}
	answer(t, typed, "n\n")
	if receive(t, second, "second Confirm") {
		t.Error("second Confirm = true, want false")
	}
}

func answer(t *testing.T, w *os.File, text string) {
	t.Helper()
	if _, err := w.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func receive(t *testing.T, ch <-chan bool, what string) bool {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatalf("%s never returned", what)
		return false
	}
}
