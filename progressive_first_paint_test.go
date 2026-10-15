package evo

import (
	"bytes"
	"sync"
	"testing"
)

// syncBuffer is a bytes.Buffer safe to read while the Output writes to it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Doing is live only (CONTRACT §2): off a terminal it writes no durable line,
// so a narrated item reaches the stream through the row that settles or the
// heartbeat, never at the call.
func TestDoing_PlainModeWritesNoLineBeforeTheTaskResolves(t *testing.T) {
	var out syncBuffer
	var payload syncBuffer
	o := initRun(Config{Name: "zq", Facades: Facades{Stdout: &payload, Stderr: &out}}, tuning{visibilityDelay: DelayForTest(0)})
	t.Cleanup(func() { _ = o.close() })

	o.Task("zq ci resolve").Doing("resolving repository root")

	if got := out.String(); got != "" {
		t.Fatalf("Doing wrote a durable line before the task resolved: %q", got)
	}
}
