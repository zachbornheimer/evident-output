package evo_test

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
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
// caller's own narrated beat has to reach the stream when it is narrated —
// not at Finish. A CLI that resolves toolchains for ten seconds before its
// first byte looks hung, and zq pins that as a 500ms wall-clock budget.
func TestDoing_PlainModeEmitsTheFirstBeatBeforeTheTaskResolves(t *testing.T) {
	var out syncBuffer
	var payload syncBuffer
	o := evo.Init(evo.Config{
		Title:           "zq",
		Isolated:        true,
		Format:          evo.FormatData,
		Stdout:          &payload,
		Stderr:          &out,
		VisibilityDelay: evo.DelayForTest(0),
	})
	t.Cleanup(func() { _ = o.Close() })

	o.Task("zq ci resolve").Doing("resolving repository root")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), "resolving repository root") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("first narrated beat never reached the stream before the task resolved; got %q", out.String())
}
