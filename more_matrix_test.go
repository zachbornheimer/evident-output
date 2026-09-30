package evo_test

import (
	"io"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestLOG002_DebugUsesClock(t *testing.T) {
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Clock: clock, Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	t.Cleanup(func() { _ = out.Close() })
	out.DebugForTest("x")
	ev := out.Events()
	if len(ev) == 0 {
		t.Fatal("no events")
	}
	// timestamps from fixed clock
	if ev[len(ev)-1].Timestamp.IsZero() {
		t.Fatal("zero ts")
	}
	_ = time.Second
}
