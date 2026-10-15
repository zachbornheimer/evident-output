package evo

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
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

// Off a terminal there is no live region to carry "I have started", so the
// first Doing call is a durable line (CONTRACT §2) that reaches the stream
// when it is narrated, not when the Task resolves. A CLI that resolves
// toolchains for ten seconds before its first byte looks hung, and zq pins
// that as a wall-clock budget.
func TestDoing_PlainModeEmitsTheFirstBeatBeforeTheTaskResolves(t *testing.T) {
	const firstBeatBudget = 500 * time.Millisecond
	var out syncBuffer
	var payload syncBuffer
	o := initRun(Config{Name: "zq", Facades: Facades{Stdout: &payload, Stderr: &out}}, tuning{visibilityDelay: DelayForTest(0)})
	t.Cleanup(func() { _ = o.close() })

	start := time.Now()
	o.Task("zq ci resolve").Doing("resolving repository root")

	for time.Since(start) < firstBeatBudget {
		if strings.Contains(out.String(), "resolving repository root") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("first narrated beat never reached the stream within %v while the task was unresolved; got %q", firstBeatBudget, out.String())
}
