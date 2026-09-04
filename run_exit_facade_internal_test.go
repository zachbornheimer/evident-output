package evo

import (
	"errors"
	"io"
	"testing"
)

// TestMain_ExitsThroughExitProcessFacade proves Main's and MainWith's only
// path to process termination is the exitProcess facade (API-018, restated
// by P6: the library still never calls os.Exit directly — Main/MainWith are
// the sole sanctioned callers of the facade, and this swaps it for a fake so
// the test process itself never actually exits).
func TestMain_ExitsThroughExitProcessFacade(t *testing.T) {
	prev := exitProcess
	defer func() { exitProcess = prev }()

	var gotCode int
	var exited bool
	exitProcess = func(code int) { gotCode = code; exited = true }

	SetDefault(Init(Config{Isolated: true, Options: []Option{To(io.Discard)}}))
	Main(func() error {
		Task("x").Done()
		return nil
	})
	if !exited {
		t.Fatal("Main did not exit through exitProcess")
	}
	if gotCode != ExitOK {
		t.Fatalf("Main exited %d, want %d", gotCode, ExitOK)
	}

	exited, gotCode = false, 0
	out := Init(Config{Isolated: true, Options: []Option{To(io.Discard)}})
	MainWith(out, func(o *Output) error {
		return errors.New("boom")
	})
	if !exited {
		t.Fatal("MainWith did not exit through exitProcess")
	}
	if gotCode != ExitFailed {
		t.Fatalf("MainWith exited %d, want %d", gotCode, ExitFailed)
	}
}
